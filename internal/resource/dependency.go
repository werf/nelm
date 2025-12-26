package resource

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/pkg/common"
)

// Represents a dependency on a Kubernetes resource in the Helm release.
type InternalDependency struct {
	*spec.ResourceMatcher

	ResourceState common.ResourceState
}

// Represents a dependency on an external resource outside of the Helm release.
type ExternalDependency struct {
	*spec.ResourceMeta
}

// Automatically detects internal dependencies on resources by examining specific fields in the
// resource spec. As an example, examining "envFrom" in a Pod container spec produces an internal
// dependency on a ConfigMap or a Secret.
func internalDependencies(unstruct *unstructured.Unstructured) []*InternalDependency {
	gvk := unstruct.GroupVersionKind()
	gk := gvk.GroupKind()

	var dependencies []*InternalDependency
	switch gk {
	case schema.GroupKind{Kind: "Deployment", Group: "apps"}:
		if pod, found := nestedMap(unstruct.Object, "spec", "template"); found {
			if deps, found := parsePod(unstruct, pod); found {
				dependencies = append(dependencies, deps...)
			}
		}
	case schema.GroupKind{Kind: "CronJob", Group: "batch"}:
		if pod, found := nestedMap(unstruct.Object, "spec", "jobTemplate", "spec", "template"); found {
			if deps, found := parsePod(unstruct, pod); found {
				dependencies = append(dependencies, deps...)
			}
		}
	case schema.GroupKind{Kind: "DaemonSet", Group: "apps"}:
		if pod, found := nestedMap(unstruct.Object, "spec", "template"); found {
			if deps, found := parsePod(unstruct, pod); found {
				dependencies = append(dependencies, deps...)
			}
		}
	case schema.GroupKind{Kind: "Job", Group: "batch"}:
		if pod, found := nestedMap(unstruct.Object, "spec", "template"); found {
			if deps, found := parsePod(unstruct, pod); found {
				dependencies = append(dependencies, deps...)
			}
		}
	case schema.GroupKind{Kind: "Pod", Group: ""}:
		if deps, found := parsePod(unstruct, unstruct.Object); found {
			dependencies = append(dependencies, deps...)
		}
	case schema.GroupKind{Kind: "ReplicaSet", Group: "apps"}:
		if pod, found := nestedMap(unstruct.Object, "spec", "template"); found {
			if deps, found := parsePod(unstruct, pod); found {
				dependencies = append(dependencies, deps...)
			}
		}
	case schema.GroupKind{Kind: "ReplicationController", Group: ""}:
		if pod, found := nestedMap(unstruct.Object, "spec", "template"); found {
			if deps, found := parsePod(unstruct, pod); found {
				dependencies = append(dependencies, deps...)
			}
		}
	case schema.GroupKind{Kind: "StatefulSet", Group: "apps"}:
		if dep, found := parseServiceName(unstruct, unstruct.Object); found {
			dependencies = append(dependencies, dep)
		}

		if pod, found := nestedMap(unstruct.Object, "spec", "template"); found {
			if deps, found := parsePod(unstruct, pod); found {
				dependencies = append(dependencies, deps...)
			}
		}
	case schema.GroupKind{Kind: "Endpoints", Group: ""}:
		// TODO(ilya-lesikov):
	case schema.GroupKind{Kind: "EndpointSlice", Group: ""}:
		// TODO(ilya-lesikov):
	case schema.GroupKind{Kind: "Ingress", Group: "networking.k8s.io"}:
		// TODO(ilya-lesikov):
	case schema.GroupKind{Kind: "IngressClass", Group: "networking.k8s.io"}:
		// TODO(ilya-lesikov):
	case schema.GroupKind{Kind: "PersistentVolumeClaim", Group: ""}:
		// TODO(ilya-lesikov):
	case schema.GroupKind{Kind: "VolumeAttachment", Group: "storage.k8s.io"}:
		// TODO(ilya-lesikov):
	case schema.GroupKind{Kind: "HorizontalPodAutoscaler", Group: "autoscaling"}:
		// TODO(ilya-lesikov):
	case schema.GroupKind{Kind: "ClusterRoleBinding", Group: "rbac.authorization.k8s.io"}, schema.GroupKind{Kind: "RoleBinding", Group: "rbac.authorization.k8s.io"}:
		if dep, found := parseRoleRef(*unstruct); found {
			dependencies = append(dependencies, dep)
		}
	}

	return dependencies
}

func parsePod(unstruct *unstructured.Unstructured, pod interface{}) (dependencies []*InternalDependency, found bool) {
	containers, _ := nestedSlice(pod, "spec", "containers")
	for _, container := range containers {
		if deps, found := parseContainer(unstruct, container); found {
			dependencies = append(dependencies, deps...)
		}
	}

	initContainers, _ := nestedSlice(pod, "spec", "initContainers")
	for _, container := range initContainers {
		if deps, found := parseContainer(unstruct, container); found {
			dependencies = append(dependencies, deps...)
		}
	}

	ephemeralContainers, _ := nestedSlice(pod, "spec", "ephemeralContainers")
	for _, container := range ephemeralContainers {
		if deps, found := parseContainer(unstruct, container); found {
			dependencies = append(dependencies, deps...)
		}
	}

	imagePullSecrets, _ := nestedSlice(pod, "spec", "imagePullSecrets")
	for _, secret := range imagePullSecrets {
		if dep, found := parseImagePullSecret(unstruct, secret); found {
			dependencies = append(dependencies, dep)
		}
	}

	if dep, found := parseNodeName(pod); found {
		dependencies = append(dependencies, dep)
	}

	if dep, found := parsePriorityClassName(unstruct, pod); found {
		dependencies = append(dependencies, dep)
	}

	resourceClaims, _ := nestedSlice(pod, "spec", "resourceClaims")
	for _, claim := range resourceClaims {
		if dep, found := parseResourceClaim(unstruct, claim); found {
			dependencies = append(dependencies, dep)
		}
	}

	if dep, found := parseRuntimeClassName(pod); found {
		dependencies = append(dependencies, dep)
	}

	if dep, found := parseServiceAccount(unstruct, pod); found {
		dependencies = append(dependencies, dep)
	}

	if dep, found := parseServiceAccountName(unstruct, pod); found {
		dependencies = append(dependencies, dep)
	}

	volumes, _ := nestedSlice(pod, "spec", "volumes")
	for _, volume := range volumes {
		if dep, found := parseVolume(unstruct, volume); found {
			dependencies = append(dependencies, dep)
		}
	}

	return dependencies, len(dependencies) > 0
}

func parseContainer(unstruct *unstructured.Unstructured, container interface{}) (dependencies []*InternalDependency, found bool) {
	envs, _ := nestedSlice(container, "env")
	for _, env := range envs {
		if dep, found := parseConfigMapKeyRef(unstruct, env); found {
			dependencies = append(dependencies, dep)
		} else if dep, found := parseSecretKeyRef(unstruct, env); found {
			dependencies = append(dependencies, dep)
		}
	}

	envFrom, _ := nestedSlice(container, "envFrom")
	for _, env := range envFrom {
		if dep, found := parseConfigMapRef(unstruct, env); found {
			dependencies = append(dependencies, dep)
		} else if dep, found := parseSecretRef(unstruct, env); found {
			dependencies = append(dependencies, dep)
		}
	}

	return dependencies, len(dependencies) > 0
}

func parseConfigMapKeyRef(unstruct *unstructured.Unstructured, env interface{}) (dep *InternalDependency, found bool) {
	configMapKeyRef, found := nestedMap(env, "valueFrom", "configMapKeyRef")
	if !found {
		return nil, false
	}

	optional, found := nestedBool(configMapKeyRef, "optional")
	if found && optional {
		return nil, false
	}

	name, found := nestedStringNotEmpty(configMapKeyRef, "name")
	if !found {
		return nil, false
	}

	dep = &InternalDependency{
		ResourceMatcher: &spec.ResourceMatcher{
			Names:      []string{name},
			Namespaces: []string{unstruct.GetNamespace()},
			Groups:     []string{""},
			Kinds:      []string{"ConfigMap"},
		},
		ResourceState: common.ResourceStatePresent,
	}

	return dep, true
}

func parseSecretKeyRef(unstruct *unstructured.Unstructured, env interface{}) (dep *InternalDependency, found bool) {
	secretKeyRef, found := nestedMap(env, "valueFrom", "secretKeyRef")
	if !found {
		return nil, false
	}

	optional, found := nestedBool(secretKeyRef, "optional")
	if found && optional {
		return nil, false
	}

	name, found := nestedStringNotEmpty(secretKeyRef, "name")
	if !found {
		return nil, false
	}

	dep = &InternalDependency{
		ResourceMatcher: &spec.ResourceMatcher{
			Names:      []string{name},
			Namespaces: []string{unstruct.GetNamespace()},
			Groups:     []string{""},
			Kinds:      []string{"Secret"},
		},
		ResourceState: common.ResourceStatePresent,
	}

	return dep, true
}

func parseConfigMapRef(unstruct *unstructured.Unstructured, env interface{}) (dep *InternalDependency, found bool) {
	configMapKeyRef, found := nestedMap(env, "valueFrom", "configMapRef")
	if !found {
		return nil, false
	}

	optional, found := nestedBool(configMapKeyRef, "optional")
	if found && optional {
		return nil, false
	}

	name, found := nestedStringNotEmpty(configMapKeyRef, "name")
	if !found {
		return nil, false
	}

	dep = &InternalDependency{
		ResourceMatcher: &spec.ResourceMatcher{
			Names:      []string{name},
			Namespaces: []string{unstruct.GetNamespace()},
			Groups:     []string{""},
			Kinds:      []string{"ConfigMap"},
		},
		ResourceState: common.ResourceStatePresent,
	}

	return dep, true
}

func parseSecretRef(unstruct *unstructured.Unstructured, env interface{}) (dep *InternalDependency, found bool) {
	secretKeyRef, found := nestedMap(env, "valueFrom", "secretRef")
	if !found {
		return nil, false
	}

	optional, found := nestedBool(secretKeyRef, "optional")
	if found && optional {
		return nil, false
	}

	name, found := nestedStringNotEmpty(secretKeyRef, "name")
	if !found {
		return nil, false
	}

	dep = &InternalDependency{
		ResourceMatcher: &spec.ResourceMatcher{
			Names:      []string{name},
			Namespaces: []string{unstruct.GetNamespace()},
			Groups:     []string{""},
			Kinds:      []string{"Secret"},
		},
		ResourceState: common.ResourceStatePresent,
	}

	return dep, true
}

func parseImagePullSecret(unstruct *unstructured.Unstructured, secret interface{}) (dep *InternalDependency, found bool) {
	name, found := nestedStringNotEmpty(secret, "name")
	if !found {
		return nil, false
	}

	dep = &InternalDependency{
		ResourceMatcher: &spec.ResourceMatcher{
			Names:      []string{name},
			Namespaces: []string{unstruct.GetNamespace()},
			Groups:     []string{""},
			Kinds:      []string{"Secret"},
		},
		ResourceState: common.ResourceStatePresent,
	}

	return dep, true
}

func parseNodeName(pod interface{}) (dep *InternalDependency, found bool) {
	nodeName, found := nestedStringNotEmpty(pod, "spec", "nodeName")
	if !found {
		return nil, false
	}

	dep = &InternalDependency{
		ResourceMatcher: &spec.ResourceMatcher{
			Names:  []string{nodeName},
			Groups: []string{""},
			Kinds:  []string{"Node"},
		},
		ResourceState: common.ResourceStatePresent,
	}

	return dep, true
}

func parsePriorityClassName(unstruct *unstructured.Unstructured, pod interface{}) (dep *InternalDependency, found bool) {
	priorityClassName, found := nestedStringNotEmpty(pod, "spec", "priorityClassName")
	if !found {
		return nil, false
	}

	dep = &InternalDependency{
		ResourceMatcher: &spec.ResourceMatcher{
			Names:  []string{priorityClassName},
			Groups: []string{"scheduling.k8s.io"},
			Kinds:  []string{"PriorityClass"},
		},
		ResourceState: common.ResourceStatePresent,
	}

	return dep, true
}

func parseResourceClaim(unstruct *unstructured.Unstructured, claim interface{}) (dep *InternalDependency, found bool) {
	source, found := nestedMap(claim, "source")
	if !found {
		return nil, false
	}

	resourceClaimName, resourceClaimNameFound := nestedStringNotEmpty(source, "resourceClaimName")
	if resourceClaimNameFound {
		dep = &InternalDependency{
			ResourceMatcher: &spec.ResourceMatcher{
				Names:      []string{resourceClaimName},
				Namespaces: []string{unstruct.GetNamespace()},
				Groups:     []string{"resource.k8s.io"},
				Kinds:      []string{"ResourceClaim"},
			},
			ResourceState: common.ResourceStatePresent,
		}

		return dep, true
	}

	resourceClaimNameTemplate, resourceClaimNameTemplateFound := nestedStringNotEmpty(source, "resourceClaimNameTemplate")
	if resourceClaimNameTemplateFound {
		dep = &InternalDependency{
			ResourceMatcher: &spec.ResourceMatcher{
				Names:      []string{resourceClaimNameTemplate},
				Namespaces: []string{unstruct.GetNamespace()},
				Groups:     []string{"resource.k8s.io"},
				Kinds:      []string{"ResourceClaimTemplate"},
			},
			ResourceState: common.ResourceStatePresent,
		}

		return dep, true
	}

	return nil, false
}

func parseRuntimeClassName(pod interface{}) (dep *InternalDependency, found bool) {
	runtimeClassName, found := nestedStringNotEmpty(pod, "spec", "runtimeClassName")
	if !found {
		return nil, false
	}

	dep = &InternalDependency{
		ResourceMatcher: &spec.ResourceMatcher{
			Names:  []string{runtimeClassName},
			Groups: []string{"node.k8s.io"},
			Kinds:  []string{"RuntimeClass"},
		},
		ResourceState: common.ResourceStatePresent,
	}

	return dep, true
}

func parseServiceAccount(unstruct *unstructured.Unstructured, pod interface{}) (dep *InternalDependency, found bool) {
	serviceAccount, found := nestedStringNotEmpty(pod, "spec", "serviceAccount")
	if !found {
		return nil, false
	}

	dep = &InternalDependency{
		ResourceMatcher: &spec.ResourceMatcher{
			Names:      []string{serviceAccount},
			Namespaces: []string{unstruct.GetNamespace()},
			Groups:     []string{""},
			Kinds:      []string{"ServiceAccount"},
		},
		ResourceState: common.ResourceStatePresent,
	}

	return dep, true
}

func parseServiceAccountName(unstruct *unstructured.Unstructured, pod interface{}) (dep *InternalDependency, found bool) {
	serviceAccountName, found := nestedStringNotEmpty(pod, "spec", "serviceAccountName")
	if !found {
		return nil, false
	}

	dep = &InternalDependency{
		ResourceMatcher: &spec.ResourceMatcher{
			Names:      []string{serviceAccountName},
			Namespaces: []string{unstruct.GetNamespace()},
			Groups:     []string{""},
			Kinds:      []string{"ServiceAccount"},
		},
		ResourceState: common.ResourceStatePresent,
	}

	return dep, true
}

func parseVolume(unstruct *unstructured.Unstructured, volume interface{}) (dep *InternalDependency, found bool) {
	configMap, found := nestedMap(volume, "configMap")
	if found {
		name, found := nestedStringNotEmpty(configMap, "name")
		if !found {
			return nil, false
		}

		optional, found := nestedBool(configMap, "optional")
		if found && optional {
			return nil, false
		}

		dep = &InternalDependency{
			ResourceMatcher: &spec.ResourceMatcher{
				Names:      []string{name},
				Namespaces: []string{unstruct.GetNamespace()},
				Groups:     []string{""},
				Kinds:      []string{"ConfigMap"},
			},
			ResourceState: common.ResourceStatePresent,
		}

		return dep, true
	}

	secret, found := nestedMap(volume, "secret")
	if found {
		name, found := nestedStringNotEmpty(secret, "secretName")
		if !found {
			return nil, false
		}

		optional, found := nestedBool(secret, "optional")
		if found && optional {
			return nil, false
		}

		dep = &InternalDependency{
			ResourceMatcher: &spec.ResourceMatcher{
				Names:      []string{name},
				Namespaces: []string{unstruct.GetNamespace()},
				Groups:     []string{""},
				Kinds:      []string{"Secret"},
			},
			ResourceState: common.ResourceStatePresent,
		}

		return dep, true
	}

	return nil, false
}

func parseServiceName(unstruct *unstructured.Unstructured, resSpec interface{}) (dep *InternalDependency, found bool) {
	name, found := nestedStringNotEmpty(resSpec, "serviceName")
	if !found {
		return nil, false
	}

	dep = &InternalDependency{
		ResourceMatcher: &spec.ResourceMatcher{
			Names:      []string{name},
			Namespaces: []string{unstruct.GetNamespace()},
			Groups:     []string{""},
			Kinds:      []string{"Service"},
		},
		ResourceState: common.ResourceStatePresent,
	}

	return dep, true
}

func parseRoleRef(unstruct unstructured.Unstructured) (dep *InternalDependency, found bool) {
	roleRef, found := nestedMap(unstruct.Object, "roleRef")
	if !found {
		return nil, false
	}

	apiGroup, found := nestedString(roleRef, "apiGroup")
	if !found {
		return nil, false
	}

	kind, found := nestedString(roleRef, "kind")
	if !found {
		return nil, false
	}

	name, found := nestedString(roleRef, "name")
	if !found {
		return nil, false
	}

	var namespaces []string
	if kind != "ClusterRole" {
		namespaces = []string{unstruct.GetNamespace()}
	}

	dep = &InternalDependency{
		ResourceMatcher: &spec.ResourceMatcher{
			Names:      []string{name},
			Namespaces: namespaces,
			Groups:     []string{apiGroup},
			Kinds:      []string{kind},
		},
		ResourceState: common.ResourceStatePresent,
	}

	return dep, true
}

func nestedSlice(object interface{}, fields ...string) (result []interface{}, found bool) {
	obj, ok := object.(map[string]interface{})
	if !ok {
		return nil, false
	}

	result, found, err := unstructured.NestedSlice(obj, fields...)
	if !found || err != nil || len(result) == 0 {
		return nil, false
	}

	return result, true
}

func nestedMap(object interface{}, fields ...string) (result map[string]interface{}, found bool) {
	obj, ok := object.(map[string]interface{})
	if !ok {
		return nil, false
	}

	result, found, err := unstructured.NestedMap(obj, fields...)
	if !found || err != nil || len(result) == 0 {
		return nil, false
	}

	return result, true
}

func nestedBool(object interface{}, fields ...string) (result, found bool) {
	obj, ok := object.(map[string]interface{})
	if !ok {
		return false, false
	}

	result, found, err := unstructured.NestedBool(obj, fields...)
	if !found || err != nil {
		return false, false
	}

	return result, true
}

func nestedString(object interface{}, fields ...string) (result string, found bool) {
	obj, ok := object.(map[string]interface{})
	if !ok {
		return "", false
	}

	result, found, err := unstructured.NestedString(obj, fields...)
	if !found || err != nil {
		return "", false
	}

	return result, true
}

func nestedStringNotEmpty(object interface{}, fields ...string) (result string, found bool) {
	obj, ok := object.(map[string]interface{})
	if !ok {
		return "", false
	}

	result, found, err := unstructured.NestedString(obj, fields...)
	if !found || err != nil || result == "" {
		return "", false
	}

	return result, true
}
