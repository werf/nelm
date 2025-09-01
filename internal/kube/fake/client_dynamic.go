package fake

import (
	"fmt"
	"reflect"

	jsonpatch "github.com/evanphx/json-patch"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	staticfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/testing"

	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource/spec"
)

func NewDynamicClient(staticClient *staticfake.Clientset, mapper meta.ResettableRESTMapper) *dynamicfake.FakeDynamicClient {
	dynClient := dynamicfake.NewSimpleDynamicClient(scheme.Scheme)
	dynClient.PrependReactor("*", "*", prepareReaction(dynClient.Tracker(), mapper))

	return dynClient
}

func prepareReaction(tracker testing.ObjectTracker, mapper meta.ResettableRESTMapper) testing.ReactionFunc {
	return func(action testing.Action) (bool, runtime.Object, error) {
		actionImpl, ok := action.(testing.PatchActionImpl)
		if !ok {
			return false, nil, nil
		}

		switch actionImpl.PatchType {
		// Default fake client doesn't support StrategicMergePatchType
		case types.StrategicMergePatchType:
			return mergePatch(actionImpl, tracker)
		// Default fake client doesn't support ApplyPatchType
		case types.ApplyPatchType:
			getObj, err := tracker.Get(actionImpl.Resource, actionImpl.Namespace, actionImpl.Name)
			if err != nil {
				if !kube.IsNotFoundErr(err) && !kube.IsNoSuchKindErr(err) {
					return true, nil, fmt.Errorf("get object for apply patch: %w", err)
				}
			}

			if getObj != nil {
				return mergePatch(actionImpl, tracker)
			} else {
				obj, gvk, err := scheme.Codecs.UniversalDecoder().Decode(actionImpl.Patch, nil, &unstructured.Unstructured{})
				if err != nil {
					return true, nil, fmt.Errorf("decode object for apply patch: %w", err)
				}

				gvr, _, err := spec.GVKtoGVR(*gvk, mapper)
				if err != nil {
					return true, nil, fmt.Errorf("map gvk to gvr for apply patch: %w", err)
				}

				if err := tracker.Create(gvr, obj, actionImpl.Namespace); err != nil {
					return true, nil, fmt.Errorf("create object for apply patch: %w", err)
				}

				return true, obj, nil
			}
		}

		return false, nil, nil
	}
}

// From: https://github.com/kubernetes/client-go/blob/d7cf8c9b31936f927a83634cc840fa2bad7368d9/testing/fixture.go#L175
func mergePatch(action testing.PatchActionImpl, tracker testing.ObjectTracker) (bool, runtime.Object, error) {
	ns := action.GetNamespace()
	gvr := action.GetResource()

	obj, err := tracker.Get(gvr, ns, action.GetName())
	if err != nil {
		return true, nil, err
	}

	old, err := json.Marshal(obj)
	if err != nil {
		return true, nil, err
	}

	value := reflect.ValueOf(obj)
	value.Elem().Set(reflect.New(value.Type().Elem()).Elem())

	modified, err := jsonpatch.MergePatch(old, action.GetPatch())
	if err != nil {
		return true, nil, err
	}

	if err := json.Unmarshal(modified, obj); err != nil {
		return true, nil, err
	}

	if err = tracker.Update(gvr, obj, ns); err != nil {
		return true, nil, err
	}

	return true, obj, nil
}
