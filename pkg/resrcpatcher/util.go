package resrcpatcher

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

func setAnnotationsAndLabels(res *unstructured.Unstructured, annotations, labels map[string]string) {
	if len(annotations) > 0 {
		annos := res.GetAnnotations()
		if annos == nil {
			annos = map[string]string{}
		}
		for k, v := range annotations {
			annos[k] = v
		}
		res.SetAnnotations(annos)
	}

	if len(labels) > 0 {
		lbls := res.GetLabels()
		if lbls == nil {
			lbls = map[string]string{}
		}
		for k, v := range labels {
			lbls[k] = v
		}
		res.SetLabels(lbls)
	}
}
