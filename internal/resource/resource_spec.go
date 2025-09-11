package resource

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/werf/nelm/internal/resource/meta"
)

type StoreAs string

const (
	StoreAsNone    StoreAs = "none"
	StoreAsHook    StoreAs = "hook"
	StoreAsRegular StoreAs = "regular"
)

type ResourceSpecOptions struct {
	StoreAs  StoreAs
	FilePath string
}

func NewResourceSpec(unstruct *unstructured.Unstructured, releaseNamespace string, opts ResourceSpecOptions) *ResourceSpec {
	if opts.StoreAs == "" {
		if IsHook(unstruct.GetAnnotations()) {
			opts.StoreAs = StoreAsHook
		} else {
			opts.StoreAs = StoreAsRegular
		}
	}

	if releaseNamespace == unstruct.GetNamespace() {
		unstruct.SetNamespace("")
	}

	return &ResourceSpec{
		ResourceMeta: meta.NewResourceMetaFromUnstructured(unstruct, releaseNamespace, opts.FilePath),
		Unstruct:     unstruct,
		StoreAs:      opts.StoreAs,
	}
}

func NewResourceSpecFromManifest(manifest string, releaseNamespace string, opts ResourceSpecOptions) (*ResourceSpec, error) {
	if opts.FilePath == "" && strings.HasPrefix(manifest, "# Source: ") {
		firstLine := strings.TrimSpace(strings.Split(manifest, "\n")[0])
		opts.FilePath = strings.TrimPrefix(firstLine, "# Source: ")
	}

	obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(manifest), nil, &unstructured.Unstructured{})
	if err != nil {
		return nil, fmt.Errorf("decode resource (file: %q): %w", opts.FilePath, err)
	}

	return NewResourceSpec(obj.(*unstructured.Unstructured), releaseNamespace, opts), nil
}

type ResourceSpec struct {
	*meta.ResourceMeta

	Unstruct *unstructured.Unstructured
	StoreAs
}
