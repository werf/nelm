package registry

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
)

// PushOptExtraTags returns a function that sets additional tags pointing to the
// pushed manifest. Plus signs are replaced with underscores, as in the main ref.
// Strict mode applies only to the main ref.
func PushOptExtraTags(tags []string) PushOption {
	return func(operation *pushOperation) {
		operation.extraTags = tags
	}
}

func normalizeExtraTags(parsedRef reference, tags []string) ([]string, error) {
	var result []string
	for _, tag := range tags {
		tag = strings.ReplaceAll(tag, "+", "_")
		if tag == parsedRef.Tag || slices.Contains(result, tag) {
			continue
		}

		tagRef := parsedRef.orasReference
		tagRef.Reference = tag
		if err := tagRef.ValidateReferenceAsTag(); err != nil {
			return nil, fmt.Errorf("validate extra tag: %w", err)
		}

		if err := ValidateChartTag(tag); err != nil {
			return nil, err
		}

		result = append(result, tag)
	}

	return result, nil
}

// ValidateChartTag rejects a tag ending in ".prov": the OCI getter treats a ref ending in ".prov"
// as a request for the provenance of the chart under the rest of the ref.
func ValidateChartTag(tag string) error {
	if strings.HasSuffix(tag, ".prov") {
		return fmt.Errorf("tag %q is not allowed: tags ending in \".prov\" address provenance files", tag)
	}

	return nil
}

func (c *Client) pushExtraTags(ctx context.Context, repository *remote.Repository, manifestDescriptor ocispec.Descriptor, parsedRef reference, pushedRef string, extraTags []string) error {
	for _, tag := range extraTags {
		if err := repository.Tag(ctx, manifestDescriptor, tag); err != nil {
			return fmt.Errorf("chart pushed as %s, but tagging it also as %q failed: %w", pushedRef, tag, err)
		}
		_, _ = fmt.Fprintf(c.out, "Tagged: %s:%s\n", parsedRef.Registry+"/"+parsedRef.Repository, tag)
	}

	return nil
}

// tagDescriptor returns the descriptor of the manifest of the tag, nil if there is no such tag.
func (c *Client) tagDescriptor(ctx context.Context, registryReference reference, tag string) (*ocispec.Descriptor, error) {
	tagReference := registryReference.orasReference
	tagReference.Reference = tag
	if tagReference.ValidateReferenceAsTag() != nil {
		return nil, nil
	}

	desc, err := c.ResolveContext(ctx, tagReference.String())
	if err != nil {
		if errors.Is(err, errdef.ErrNotFound) {
			return nil, nil
		}

		return nil, err
	}

	return &desc, nil
}

// ClientOptBasicAuthHosts limits the username and password to the given registry hosts (host or
// host:port). Other hosts get the credentials from the credentials store. Docker Hub names are
// matched to the host the requests go to.
func ClientOptBasicAuthHosts(hosts ...string) ClientOption {
	return func(client *Client) {
		client.basicAuthHosts = make([]string, 0, len(hosts))
		for _, host := range hosts {
			if host == "index.docker.io" {
				host = "docker.io"
			}

			client.basicAuthHosts = append(client.basicAuthHosts, registry.Reference{Registry: host}.Host())
		}
	}
}

// ClientOptPlainHTTPHost makes the client decide per registry host (host or host:port) whether to
// use plain HTTP, instead of following ClientOptPlainHTTP.
func ClientOptPlainHTTPHost(plainHTTPHost func(host string) bool) ClientOption {
	return func(client *Client) {
		client.plainHTTPHost = plainHTTPHost
	}
}

// Repository returns the remote repository of ref, authenticated and configured like the client.
func (c *GenericClient) Repository(ref string) (*remote.Repository, error) {
	repository, err := remote.NewRepository(ref)
	if err != nil {
		return nil, fmt.Errorf("parse repository reference %q: %w", ref, err)
	}
	repository.PlainHTTP = isPlainHTTP(c.plainHTTP, c.plainHTTPHost, repository.Reference.Registry)
	repository.Client = c.authorizer

	return repository, nil
}

func (c *Client) basicAuthHostsCredential() auth.CredentialFunc {
	storeCredential := credentials.Credential(c.credentialsStore)

	return func(ctx context.Context, hostport string) (auth.Credential, error) {
		if slices.Contains(c.basicAuthHosts, hostport) {
			return auth.Credential{Username: c.username, Password: c.password}, nil
		}

		return storeCredential(ctx, hostport)
	}
}

// isPlainHTTP returns whether to use plain HTTP with the registry host.
func isPlainHTTP(plainHTTP bool, plainHTTPHost func(host string) bool, host string) bool {
	if plainHTTPHost != nil {
		return plainHTTPHost(host)
	}

	return plainHTTP
}
