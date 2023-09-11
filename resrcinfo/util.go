package resrcinfo

import (
	"strings"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func isImmutableErr(err error) bool {
	return err != nil && errors.IsInvalid(err) && strings.Contains(err.Error(), "field is immutable")
}

func isNoSuchKindErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no matches for kind")
}

func isNotFoundErr(err error) bool {
	return err != nil && errors.IsNotFound(err)
}

func diffGetAndDryApplyObjects(getObj *unstructured.Unstructured, dryApplyObj *unstructured.Unstructured, cmpOptions ...cmp.Option) bool {
	ignoreFilter := func(path cmp.Path) bool {
		switch path.GoString() {
		case
			`{map[string]any}["metadata"].(map[string]any)["creationTimestamp"]`,
			`{map[string]any}["metadata"].(map[string]any)["generation"]`,
			`{map[string]any}["metadata"].(map[string]any)["resourceVersion"]`,
			`{map[string]any}["metadata"].(map[string]any)["uid"]`,
			`{map[string]any}["metadata"].(map[string]any)["managedFields"]`,
			`{map[string]any}["status"]`:
			return true
		}

		if after, found := strings.CutPrefix(path.GoString(), `{map[string]any}["metadata"].(map[string]any)["annotations"].(map[string]any)["`); found {
			annoKey, _, _ := strings.Cut(after, `"`)
			if strings.Contains(annoKey, "werf.io") {
				return true
			}
		}

		if after, found := strings.CutPrefix(path.GoString(), `{map[string]any}["metadata"].(map[string]any)["labels"].(map[string]any)["`); found {
			annoKey, _, _ := strings.Cut(after, `"`)
			if strings.Contains(annoKey, "werf.io") {
				return true
			}
		}

		return false
	}
	opts := append([]cmp.Option{cmp.FilterPath(ignoreFilter, cmp.Ignore())}, cmpOptions...)

	different := !cmp.Equal(getObj.UnstructuredContent(), dryApplyObj.UnstructuredContent(), opts...)

	return different
}
