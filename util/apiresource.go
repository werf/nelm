package util

import (
	"fmt"
	"strings"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func ResourceFromAPIResourcesByGVK(listOfApiResourceLists []*v1.APIResourceList, gvk schema.GroupVersionKind) (*v1.APIResource, error) {
	var result *v1.APIResource

	for _, apiResourceList := range listOfApiResourceLists {
		if apiResourceList.GroupVersion == gvk.GroupVersion().String() {
			for _, apiRes := range apiResourceList.APIResources {
				if apiRes.Kind == gvk.Kind && !strings.Contains(apiRes.Name, "/") {
					result = &apiRes
					break
				}
			}
		}
	}

	if result == nil {
		return nil, fmt.Errorf("no api resource found for groupVersionKind %q", gvk.String())
	}

	return result, nil
}
