package meta

import (
	"fmt"
	"strings"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
)

func NewResourceMeta(name, namespace, releaseNamespace, filePath string, gvk schema.GroupVersionKind, annotations, labels map[string]string) *ResourceMeta {
	if releaseNamespace == namespace {
		namespace = ""
	}

	return &ResourceMeta{
		Name:             name,
		Namespace:        namespace,
		GroupVersionKind: gvk,
		FilePath:         filePath,
		Annotations:      annotations,
		Labels:           labels,
	}
}

func NewResourceMetaFromUnstructured(unstruct *unstructured.Unstructured, releaseNamespace, filePath string) *ResourceMeta {
	return NewResourceMeta(unstruct.GetName(), unstruct.GetNamespace(), releaseNamespace, filePath, unstruct.GroupVersionKind(), unstruct.GetAnnotations(), unstruct.GetLabels())
}

func NewResourceMetaFromPartialMetadata(meta *v1.PartialObjectMetadata, releaseNamespace, filePath string) *ResourceMeta {
	return NewResourceMeta(meta.GetName(), meta.GetNamespace(), releaseNamespace, filePath, meta.GroupVersionKind(), meta.GetAnnotations(), meta.GetLabels())
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

type ResourceMeta struct {
	Name             string
	Namespace        string
	GroupVersionKind schema.GroupVersionKind
	FilePath         string
	Annotations      map[string]string
	Labels           map[string]string
}

func (m *ResourceMeta) ID() string {
	return fmt.Sprintf("%s:%s:%s:%s", m.Namespace, m.GroupVersionKind.Group, m.GroupVersionKind.Kind, m.Name)
}

func (m *ResourceMeta) IDWithVersion() string {
	return fmt.Sprintf("%s:%s:%s:%s:%s", m.Namespace, m.GroupVersionKind.Group, m.GroupVersionKind.Version, m.GroupVersionKind.Kind, m.Name)
}

func (m *ResourceMeta) IDHuman() string {
	id := fmt.Sprintf("%s/%s", m.GroupVersionKind.Kind, m.Name)

	if m.Namespace != "" {
		id = fmt.Sprintf("%s (namespace=%s)", id, m.Namespace)
	}

	return id
}
