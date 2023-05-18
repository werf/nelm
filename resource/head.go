package resource

import (
	"fmt"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/releaseutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewHead() *Head {
	return &Head{}
}

type Head struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
}

// YAML multi-documents allowed.
func ResourceHeadsFromManifests(manifests ...string) ([]*Head, error) {
	var multidoc string
	for _, manifest := range manifests {
		multidoc = fmt.Sprintf("%s\n---\n%s", multidoc, manifest)
	}

	var splitManifests []string
	for _, manifest := range releaseutil.SplitManifests(multidoc) {
		splitManifests = append(splitManifests, manifest)
	}

	var heads []*Head
	for _, manifest := range splitManifests {
		head := NewHead()
		if err := yaml.Unmarshal([]byte(manifest), head); err != nil {
			return nil, fmt.Errorf("error unmarshalling manifest: %w", err)
		}
		heads = append(heads, head)
	}

	return heads, nil
}
