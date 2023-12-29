package depnddetctr

import (
	"helm.sh/helm/v3/pkg/werf/depnd"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func NewInternalDependencyDetector(opts InternalDependencyDetectorOptions) *InternalDependencyDetector {
	return &InternalDependencyDetector{
		defaultNamespace: opts.DefaultNamespace,
	}
}

type InternalDependencyDetectorOptions struct {
	DefaultNamespace string
}

type InternalDependencyDetector struct {
	defaultNamespace string
}

func (d *InternalDependencyDetector) Detect(unstruct *unstructured.Unstructured) []*depnd.InternalDependency {
	gvk := unstruct.GroupVersionKind()
	gk := gvk.GroupKind()

	var dependencies []*depnd.InternalDependency
	if gk == (schema.GroupKind{Kind: "Deployment", Group: "apps"}) {
		if pod, found := nestedMap(unstruct.Object, "spec", "template"); found {
			if deps, found := d.parsePod(unstruct, pod); found {
				dependencies = append(dependencies, deps...)
			}
		}
	} else if gk == (schema.GroupKind{Kind: "CronJob", Group: "batch"}) {
		if pod, found := nestedMap(unstruct.Object, "spec", "jobTemplate", "spec", "template"); found {
			if deps, found := d.parsePod(unstruct, pod); found {
				dependencies = append(dependencies, deps...)
			}
		}
	} else if gk == (schema.GroupKind{Kind: "DaemonSet", Group: "apps"}) {
		if pod, found := nestedMap(unstruct.Object, "spec", "template"); found {
			if deps, found := d.parsePod(unstruct, pod); found {
				dependencies = append(dependencies, deps...)
			}
		}
	} else if gk == (schema.GroupKind{Kind: "Job", Group: "batch"}) {
		if pod, found := nestedMap(unstruct.Object, "spec", "template"); found {
			if deps, found := d.parsePod(unstruct, pod); found {
				dependencies = append(dependencies, deps...)
			}
		}
	} else if gk == (schema.GroupKind{Kind: "Pod", Group: ""}) {
		if deps, found := d.parsePod(unstruct, unstruct.Object); found {
			dependencies = append(dependencies, deps...)
		}
	} else if gk == (schema.GroupKind{Kind: "ReplicaSet", Group: "apps"}) {
		if pod, found := nestedMap(unstruct.Object, "spec", "template"); found {
			if deps, found := d.parsePod(unstruct, pod); found {
				dependencies = append(dependencies, deps...)
			}
		}
	} else if gk == (schema.GroupKind{Kind: "ReplicationController", Group: ""}) {
		if pod, found := nestedMap(unstruct.Object, "spec", "template"); found {
			if deps, found := d.parsePod(unstruct, pod); found {
				dependencies = append(dependencies, deps...)
			}
		}
	} else if gk == (schema.GroupKind{Kind: "StatefulSet", Group: "apps"}) {
		if dep, found := d.parseServiceName(unstruct, unstruct.Object); found {
			dependencies = append(dependencies, dep)
		}

		if pod, found := nestedMap(unstruct.Object, "spec", "template"); found {
			if deps, found := d.parsePod(unstruct, pod); found {
				dependencies = append(dependencies, deps...)
			}
		}
	} else if gk == (schema.GroupKind{Kind: "Endpoints", Group: ""}) {
		// 	TODO(ilya-lesikov):
	} else if gk == (schema.GroupKind{Kind: "EndpointSlice", Group: ""}) {
		// 	TODO(ilya-lesikov):
	} else if gk == (schema.GroupKind{Kind: "Ingress", Group: "networking.k8s.io"}) {
		// 	TODO(ilya-lesikov):
	} else if gk == (schema.GroupKind{Kind: "IngressClass", Group: "networking.k8s.io"}) {
		// 	TODO(ilya-lesikov):
	} else if gk == (schema.GroupKind{Kind: "PersistentVolumeClaim", Group: ""}) {
		// 	TODO(ilya-lesikov):
	} else if gk == (schema.GroupKind{Kind: "VolumeAttachment", Group: "storage.k8s.io"}) {
		// TODO(ilya-lesikov):
	} else if gk == (schema.GroupKind{Kind: "HorizontalPodAutoscaler", Group: "autoscaling"}) {
		// TODO(ilya-lesikov):
	} else if gk == (schema.GroupKind{Kind: "ClusterRoleBinding", Group: "rbac.authorization.k8s.io"}) {
		if dep, found := d.parseRoleRef(*unstruct); found {
			dependencies = append(dependencies, dep)
		}
	} else if gk == (schema.GroupKind{Kind: "RoleBinding", Group: "rbac.authorization.k8s.io"}) {
		if dep, found := d.parseRoleRef(*unstruct); found {
			dependencies = append(dependencies, dep)
		}
	}

	return dependencies
}

func (d *InternalDependencyDetector) parsePod(unstruct *unstructured.Unstructured, pod interface{}) (dependencies []*depnd.InternalDependency, found bool) {
	containers, _ := nestedSlice(pod, "spec", "containers")
	for _, container := range containers {
		if deps, found := d.parseContainer(unstruct, container); found {
			dependencies = append(dependencies, deps...)
		}
	}

	initContainers, _ := nestedSlice(pod, "spec", "initContainers")
	for _, container := range initContainers {
		if deps, found := d.parseContainer(unstruct, container); found {
			dependencies = append(dependencies, deps...)
		}
	}

	ephemeralContainers, _ := nestedSlice(pod, "spec", "ephemeralContainers")
	for _, container := range ephemeralContainers {
		if deps, found := d.parseContainer(unstruct, container); found {
			dependencies = append(dependencies, deps...)
		}
	}

	imagePullSecrets, _ := nestedSlice(pod, "spec", "imagePullSecrets")
	for _, secret := range imagePullSecrets {
		if dep, found := d.parseImagePullSecret(unstruct, secret); found {
			dependencies = append(dependencies, dep)
		}
	}

	if dep, found := d.parseNodeName(pod); found {
		dependencies = append(dependencies, dep)
	}

	if dep, found := d.parsePriorityClassName(unstruct, pod); found {
		dependencies = append(dependencies, dep)
	}

	resourceClaims, _ := nestedSlice(pod, "spec", "resourceClaims")
	for _, claim := range resourceClaims {
		if dep, found := d.parseResourceClaim(unstruct, claim); found {
			dependencies = append(dependencies, dep)
		}
	}

	if dep, found := d.parseRuntimeClassName(pod); found {
		dependencies = append(dependencies, dep)
	}

	if dep, found := d.parseServiceAccount(unstruct, pod); found {
		dependencies = append(dependencies, dep)
	}

	if dep, found := d.parseServiceAccountName(unstruct, pod); found {
		dependencies = append(dependencies, dep)
	}

	volumes, _ := nestedSlice(pod, "spec", "volumes")
	for _, volume := range volumes {
		if dep, found := d.parseVolume(unstruct, volume); found {
			dependencies = append(dependencies, dep)
		}
	}

	return dependencies, len(dependencies) > 0
}

func (d *InternalDependencyDetector) parseContainer(unstruct *unstructured.Unstructured, container interface{}) (dependencies []*depnd.InternalDependency, found bool) {
	envs, _ := nestedSlice(container, "env")
	for _, env := range envs {
		if dep, found := d.parseConfigMapKeyRef(unstruct, env); found {
			dependencies = append(dependencies, dep)
		} else if dep, found := d.parseSecretKeyRef(unstruct, env); found {
			dependencies = append(dependencies, dep)
		}
	}

	envFrom, _ := nestedSlice(container, "envFrom")
	for _, env := range envFrom {
		if dep, found := d.parseConfigMapRef(unstruct, env); found {
			dependencies = append(dependencies, dep)
		} else if dep, found := d.parseSecretRef(unstruct, env); found {
			dependencies = append(dependencies, dep)
		}
	}

	return dependencies, len(dependencies) > 0
}

func (d *InternalDependencyDetector) parseConfigMapKeyRef(unstruct *unstructured.Unstructured, env interface{}) (dep *depnd.InternalDependency, found bool) {
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

	dep = depnd.NewInternalDependency(
		[]string{name},
		[]string{d.namespace(unstruct)},
		[]string{""},
		[]string{},
		[]string{"ConfigMap"},
		depnd.InternalDependencyOptions{
			DefaultNamespace: d.defaultNamespace,
		},
	)

	return dep, true
}

func (d *InternalDependencyDetector) parseSecretKeyRef(unstruct *unstructured.Unstructured, env interface{}) (dep *depnd.InternalDependency, found bool) {
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

	dep = depnd.NewInternalDependency(
		[]string{name},
		[]string{d.namespace(unstruct)},
		[]string{""},
		[]string{},
		[]string{"Secret"},
		depnd.InternalDependencyOptions{
			DefaultNamespace: d.defaultNamespace,
		},
	)

	return dep, true
}

func (d *InternalDependencyDetector) parseConfigMapRef(unstruct *unstructured.Unstructured, env interface{}) (dep *depnd.InternalDependency, found bool) {
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

	dep = depnd.NewInternalDependency(
		[]string{name},
		[]string{d.namespace(unstruct)},
		[]string{""},
		[]string{},
		[]string{"ConfigMap"},
		depnd.InternalDependencyOptions{
			DefaultNamespace: d.defaultNamespace,
		},
	)

	return dep, true
}

func (d *InternalDependencyDetector) parseSecretRef(unstruct *unstructured.Unstructured, env interface{}) (dep *depnd.InternalDependency, found bool) {
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

	dep = depnd.NewInternalDependency(
		[]string{name},
		[]string{d.namespace(unstruct)},
		[]string{""},
		[]string{},
		[]string{"Secret"},
		depnd.InternalDependencyOptions{
			DefaultNamespace: d.defaultNamespace,
		},
	)

	return dep, true
}

func (d *InternalDependencyDetector) parseImagePullSecret(unstruct *unstructured.Unstructured, secret interface{}) (dep *depnd.InternalDependency, found bool) {
	name, found := nestedStringNotEmpty(secret, "name")
	if !found {
		return nil, false
	}

	dep = depnd.NewInternalDependency(
		[]string{name},
		[]string{d.namespace(unstruct)},
		[]string{""},
		[]string{},
		[]string{"Secret"},
		depnd.InternalDependencyOptions{
			DefaultNamespace: d.defaultNamespace,
		},
	)

	return dep, true
}

func (d *InternalDependencyDetector) parseNodeName(pod interface{}) (dep *depnd.InternalDependency, found bool) {
	nodeName, found := nestedStringNotEmpty(pod, "spec", "nodeName")
	if !found {
		return nil, false
	}

	dep = depnd.NewInternalDependency(
		[]string{nodeName},
		[]string{},
		[]string{""},
		[]string{},
		[]string{"Node"},
		depnd.InternalDependencyOptions{
			DefaultNamespace: d.defaultNamespace,
		},
	)

	return dep, true
}

func (d *InternalDependencyDetector) parsePriorityClassName(unstruct *unstructured.Unstructured, pod interface{}) (dep *depnd.InternalDependency, found bool) {
	priorityClassName, found := nestedStringNotEmpty(pod, "spec", "priorityClassName")
	if !found {
		return nil, false
	}

	dep = depnd.NewInternalDependency(
		[]string{priorityClassName},
		[]string{},
		[]string{"scheduling.k8s.io"},
		[]string{},
		[]string{"PriorityClass"},
		depnd.InternalDependencyOptions{
			DefaultNamespace: d.defaultNamespace,
		},
	)

	return dep, true
}

func (d *InternalDependencyDetector) parseResourceClaim(unstruct *unstructured.Unstructured, claim interface{}) (dep *depnd.InternalDependency, found bool) {
	source, found := nestedMap(claim, "source")
	if !found {
		return nil, false
	}

	resourceClaimName, resourceClaimNameFound := nestedStringNotEmpty(source, "resourceClaimName")
	if resourceClaimNameFound {
		dep = depnd.NewInternalDependency(
			[]string{resourceClaimName},
			[]string{d.namespace(unstruct)},
			[]string{"resource.k8s.io"},
			[]string{},
			[]string{"ResourceClaim"},
			depnd.InternalDependencyOptions{
				DefaultNamespace: d.defaultNamespace,
			},
		)

		return dep, true
	}

	resourceClaimNameTemplate, resourceClaimNameTemplateFound := nestedStringNotEmpty(source, "resourceClaimNameTemplate")
	if resourceClaimNameTemplateFound {
		dep = depnd.NewInternalDependency(
			[]string{resourceClaimNameTemplate},
			[]string{d.namespace(unstruct)},
			[]string{"resource.k8s.io"},
			[]string{},
			[]string{"ResourceClaimTemplate"},
			depnd.InternalDependencyOptions{
				DefaultNamespace: d.defaultNamespace,
			},
		)

		return dep, true
	}

	return nil, false
}

func (d *InternalDependencyDetector) parseRuntimeClassName(pod interface{}) (dep *depnd.InternalDependency, found bool) {
	runtimeClassName, found := nestedStringNotEmpty(pod, "spec", "runtimeClassName")
	if !found {
		return nil, false
	}

	dep = depnd.NewInternalDependency(
		[]string{runtimeClassName},
		[]string{},
		[]string{"node.k8s.io"},
		[]string{},
		[]string{"RuntimeClass"},
		depnd.InternalDependencyOptions{
			DefaultNamespace: d.defaultNamespace,
		},
	)

	return dep, true
}

func (d *InternalDependencyDetector) parseServiceAccount(unstruct *unstructured.Unstructured, pod interface{}) (dep *depnd.InternalDependency, found bool) {
	serviceAccount, found := nestedStringNotEmpty(pod, "spec", "serviceAccount")
	if !found {
		return nil, false
	}

	dep = depnd.NewInternalDependency(
		[]string{serviceAccount},
		[]string{d.namespace(unstruct)},
		[]string{""},
		[]string{},
		[]string{"ServiceAccount"},
		depnd.InternalDependencyOptions{
			DefaultNamespace: d.defaultNamespace,
		},
	)

	return dep, true
}

func (d *InternalDependencyDetector) parseServiceAccountName(unstruct *unstructured.Unstructured, pod interface{}) (dep *depnd.InternalDependency, found bool) {
	serviceAccountName, found := nestedStringNotEmpty(pod, "spec", "serviceAccountName")
	if !found {
		return nil, false
	}

	dep = depnd.NewInternalDependency(
		[]string{serviceAccountName},
		[]string{d.namespace(unstruct)},
		[]string{""},
		[]string{},
		[]string{"ServiceAccount"},
		depnd.InternalDependencyOptions{
			DefaultNamespace: d.defaultNamespace,
		},
	)

	return dep, true
}

func (d *InternalDependencyDetector) parseVolume(unstruct *unstructured.Unstructured, volume interface{}) (dep *depnd.InternalDependency, found bool) {
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

		dep = depnd.NewInternalDependency(
			[]string{name},
			[]string{d.namespace(unstruct)},
			[]string{""},
			[]string{},
			[]string{"ConfigMap"},
			depnd.InternalDependencyOptions{
				DefaultNamespace: d.defaultNamespace,
			},
		)

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

		dep = depnd.NewInternalDependency(
			[]string{name},
			[]string{d.namespace(unstruct)},
			[]string{""},
			[]string{},
			[]string{"Secret"},
			depnd.InternalDependencyOptions{
				DefaultNamespace: d.defaultNamespace,
			},
		)

		return dep, true
	}

	return nil, false
}

func (d *InternalDependencyDetector) parseServiceName(unstruct *unstructured.Unstructured, spec interface{}) (dep *depnd.InternalDependency, found bool) {
	name, found := nestedStringNotEmpty(spec, "serviceName")
	if !found {
		return nil, false
	}

	dep = depnd.NewInternalDependency(
		[]string{name},
		[]string{d.namespace(unstruct)},
		[]string{""},
		[]string{},
		[]string{"Service"},
		depnd.InternalDependencyOptions{
			DefaultNamespace: d.defaultNamespace,
		},
	)

	return dep, true
}

func (d *InternalDependencyDetector) parseRoleRef(unstruct unstructured.Unstructured) (dep *depnd.InternalDependency, found bool) {
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

	dep = depnd.NewInternalDependency(
		[]string{name},
		[]string{d.namespace(&unstruct)},
		[]string{apiGroup},
		[]string{},
		[]string{kind},
		depnd.InternalDependencyOptions{
			DefaultNamespace: d.defaultNamespace,
		},
	)

	return dep, true
}

func (d *InternalDependencyDetector) namespace(unstruct *unstructured.Unstructured) string {
	if unstruct.GetNamespace() != "" {
		return unstruct.GetNamespace()
	} else if d.defaultNamespace != "" {
		return d.defaultNamespace
	}

	return v1.NamespaceDefault
}

func nestedSlice(obj interface{}, fields ...string) (result []interface{}, found bool) {
	result, found, err := unstructured.NestedSlice(obj.(map[string]interface{}), fields...)
	if !found || err != nil || len(result) == 0 {
		return nil, false
	}

	return result, true
}

func nestedMap(obj interface{}, fields ...string) (result map[string]interface{}, found bool) {
	result, found, err := unstructured.NestedMap(obj.(map[string]interface{}), fields...)
	if !found || err != nil || len(result) == 0 {
		return nil, false
	}

	return result, true
}

func nestedBool(obj interface{}, fields ...string) (result bool, found bool) {
	result, found, err := unstructured.NestedBool(obj.(map[string]interface{}), fields...)
	if !found || err != nil {
		return false, false
	}

	return result, true
}

func nestedString(obj interface{}, fields ...string) (result string, found bool) {
	result, found, err := unstructured.NestedString(obj.(map[string]interface{}), fields...)
	if !found || err != nil {
		return "", false
	}

	return result, true
}

func nestedStringNotEmpty(obj interface{}, fields ...string) (result string, found bool) {
	result, found, err := unstructured.NestedString(obj.(map[string]interface{}), fields...)
	if !found || err != nil || result == "" {
		return "", false
	}

	return result, true
}
