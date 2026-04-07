package spec

import (
	"fmt"
	"strings"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
)

// Contains basic information about a Kubernetes resource, without its full spec. Very useful for
// getting, deleting, tracking resources, when you don't care about the resource spec (or when it's
// not available). If also the resource spec is needed, use ResourceSpec.
type ResourceMeta struct {
	Name             string                  `json:"name"`
	Namespace        string                  `json:"namespace"`
	GroupVersionKind schema.GroupVersionKind `json:"groupVersionKind"`
	FilePath         string                  `json:"filePath"`
	Annotations      map[string]string       `json:"annotations"`
	Labels           map[string]string       `json:"labels"`
}

func NewResourceMeta(name, namespace, releaseNamespace, filePath string, gvk schema.GroupVersionKind, annotations, labels map[string]string) *ResourceMeta {
	if releaseNamespace == namespace {
		namespace = ""
	}

	if annotations == nil {
		annotations = map[string]string{}
	}

	if labels == nil {
		labels = map[string]string{}
	}

	return &ResourceMeta{
		Annotations:      annotations,
		FilePath:         filePath,
		GroupVersionKind: gvk,
		Labels:           labels,
		Name:             name,
		Namespace:        namespace,
	}
}

func NewResourceMetaFromManifest(manifest, releaseNamespace string) (*ResourceMeta, error) {
	var filePath string
	if strings.HasPrefix(manifest, "# Source: ") {
		firstLine := strings.TrimSpace(strings.Split(manifest, "\n")[0])
		filePath = strings.TrimPrefix(firstLine, "# Source: ")
	}

	obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(manifest), nil, &v1.PartialObjectMetadata{})
	if err != nil {
		return nil, fmt.Errorf("decode resource (file: %q): %w", filePath, err)
	}

	return NewResourceMetaFromPartialMetadata(obj.(*v1.PartialObjectMetadata), releaseNamespace, filePath), nil
}

func NewResourceMetaFromPartialMetadata(meta *v1.PartialObjectMetadata, releaseNamespace, filePath string) *ResourceMeta {
	return NewResourceMeta(meta.GetName(), meta.GetNamespace(), releaseNamespace, filePath, meta.GroupVersionKind(), meta.GetAnnotations(), meta.GetLabels())
}

func NewResourceMetaFromUnstructured(unstruct *unstructured.Unstructured, releaseNamespace, filePath string) *ResourceMeta {
	return NewResourceMeta(unstruct.GetName(), unstruct.GetNamespace(), releaseNamespace, filePath, unstruct.GroupVersionKind(), unstruct.GetAnnotations(), unstruct.GetLabels())
}

// Uniquely identifies the resource.
func (m *ResourceMeta) ID() string {
	return ID(m.Name, m.Namespace, m.GroupVersionKind.Group, m.GroupVersionKind.Kind)
}

func (m *ResourceMeta) IDHuman() string {
	return IDHuman(m.Name, m.Namespace, m.GroupVersionKind.Group, m.GroupVersionKind.Kind)
}

func (m *ResourceMeta) IDWithVersion() string {
	return IDWithVersion(m.Name, m.Namespace, m.GroupVersionKind.Group, m.GroupVersionKind.Version, m.GroupVersionKind.Kind)
}

func ID(name, namespace, group, kind string) string {
	return fmt.Sprintf("%s:%s:%s:%s", namespace, group, kind, name)
}

func IDHuman(name, namespace, group, kind string) string {
	id := fmt.Sprintf("%s/%s", kind, name)

	if namespace != "" {
		id = fmt.Sprintf("%s (namespace=%s)", id, namespace)
	}

	return id
}

func IDWithVersion(name, namespace, group, version, kind string) string {
	return fmt.Sprintf("%s:%s:%s:%s:%s", namespace, group, version, kind, name)
}
