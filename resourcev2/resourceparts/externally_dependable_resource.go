package resourceparts

import (
	"fmt"
	"regexp"
	"strings"

	"helm.sh/helm/v3/pkg/werf/errors"
	"helm.sh/helm/v3/pkg/werf/externaldependency"
	"helm.sh/helm/v3/pkg/werf/util"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
)

var annotationKeyPatternExternalDependencyResource = regexp.MustCompile(`^(?P<id>.+).external-dependency.werf.io/resource$`)
var annotationKeyPatternExternalDependencyNamespace = regexp.MustCompile(`^(?P<id>.+).external-dependency.werf.io/namespace$`)

func NewExternallyDependableResource(unstruct *unstructured.Unstructured, filePath string, opts NewExternallyDependableResourceOptions) *ExternallyDependableResource {
	return &ExternallyDependableResource{
		unstructured:    unstruct,
		mapper:          opts.Mapper,
		discoveryClient: opts.DiscoveryClient,
	}
}

type NewExternallyDependableResourceOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}

type ExternallyDependableResource struct {
	unstructured    *unstructured.Unstructured
	filePath        string
	mapper          meta.ResettableRESTMapper
	discoveryClient discovery.CachedDiscoveryInterface
}

func (r *ExternallyDependableResource) Validate() error {
	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternExternalDependencyResource); found {
		for key, value := range annotations {
			keyMatches := annotationKeyPatternExternalDependencyResource.FindStringSubmatch(key)
			if keyMatches == nil {
				return errors.NewValidationError("invalid key for annotation %q", key)
			}

			idSubexpIndex := annotationKeyPatternExternalDependencyResource.SubexpIndex("id")
			if idSubexpIndex == -1 {
				return errors.NewValidationError("invalid regexp pattern %q for annotation %q", annotationKeyPatternExternalDependencyResource.String(), key)
			}

			if len(keyMatches) < idSubexpIndex+1 {
				return errors.NewValidationError("can't parse external dependency id from annotation key %q", key)
			}

			valueElems := strings.Split(value, "/")

			if len(valueElems) != 2 {
				return errors.NewValidationError(`invalid format of value %q for annotation %q, should be: type/name`, value, key)
			}

			switch valueElems[0] {
			case "":
				return errors.NewValidationError("value %q of annotation %q can't have empty resource type", value, key)
			case "all":
				return errors.NewValidationError(`"all" resource type in value %q of annotation %q is not allowed`, value, key)
			}

			resourceTypeParts := strings.Split(valueElems[0], ".")
			for _, part := range resourceTypeParts {
				if part == "" {
					return errors.NewValidationError("resource type in value %q of annotation %q should have dots (.) delimiting only non-empty resource.version.group", value, key)
				}
			}

			if valueElems[1] == "" {
				return errors.NewValidationError("in value %q of annotation %q resource name can't be empty", value, key)
			}
		}
	}

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternExternalDependencyNamespace); found {
		for key, value := range annotations {
			keyMatches := annotationKeyPatternExternalDependencyNamespace.FindStringSubmatch(key)
			if keyMatches == nil {
				return errors.NewValidationError("invalid key for annotation %q", key)
			}

			idSubexpIndex := annotationKeyPatternExternalDependencyNamespace.SubexpIndex("id")
			if idSubexpIndex == -1 {
				return errors.NewValidationError("invalid regexp pattern %q for annotation %q", annotationKeyPatternExternalDependencyNamespace.String(), key)
			}

			if len(keyMatches) < idSubexpIndex+1 {
				return errors.NewValidationError("can't parse external dependency id from annotation key %q", key)
			}

			if value == "" {
				return errors.NewValidationError("invalid value %q for annotation %q, value must not be empty", value, key)
			}

			// TODO(ilya-lesikov): validate namespace name
		}
	}

	return nil
}

// FIXME(ilya-lesikov): don't forget to validate
func (r *ExternallyDependableResource) ExternalDependencies() ([]*externaldependency.ExternalDependency, error) {
	// Pretend we don't have any external dependencies if we don't have cluster access to map GroupVersionResource to GroupVersionKind.
	if r.mapper == nil || r.discoveryClient == nil {
		return nil, nil
	}

	type ExtDepInfo struct {
		Name      string
		Namespace string
		Type      string
	}

	extDepInfos := map[string]*ExtDepInfo{}
	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternExternalDependencyResource); found {
		for key, value := range annotations {
			matches := annotationKeyPatternExternalDependencyResource.FindStringSubmatch(key)
			idSubexpIndex := annotationKeyPatternExternalDependencyResource.SubexpIndex("id")
			extDepID := matches[idSubexpIndex]
			extDepType := strings.Split(value, "/")[0]
			extDepName := strings.Split(value, "/")[1]

			extDepInfos[extDepID] = &ExtDepInfo{
				Name: extDepName,
				Type: extDepType,
			}
		}
	}

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternExternalDependencyNamespace); found {
		for key, value := range annotations {
			matches := annotationKeyPatternExternalDependencyNamespace.FindStringSubmatch(key)
			idSubexpIndex := annotationKeyPatternExternalDependencyNamespace.SubexpIndex("id")
			extDepID := matches[idSubexpIndex]
			extDepNamespace := value

			if extDepInfo, hasKey := extDepInfos[extDepID]; hasKey {
				extDepInfo.Namespace = extDepNamespace
			}
		}
	}

	var extDeps []*externaldependency.ExternalDependency
	for _, extDepInfo := range extDepInfos {
		gvk, err := util.ParseResourceStringtoGVK(extDepInfo.Type, r.mapper, r.discoveryClient)
		if err != nil {
			return nil, fmt.Errorf("error parsing external dependency resource type %q: %w", extDepInfo.Type, err)
		}

		extDep := externaldependency.NewExternalDependency(extDepInfo.Name, r.filePath, gvk, r.mapper, externaldependency.NewExternalDependencyOptions{extDepInfo.Namespace})
		extDeps = append(extDeps, extDep)
	}

	return extDeps, nil
}
