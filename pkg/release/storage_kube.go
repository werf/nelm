package release

import (
	"context"
	"fmt"
	"maps"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
)

const (
	kubeStorageKindConfigMap kubeStorageKind = "configmap"
	kubeStorageKindSecret    kubeStorageKind = "secret"

	kubeStorageDataKey  = "release"
	kubeStoragePageSize = 500
)

var _ storageBackend = (*kubeStorageBackend)(nil)

type kubeStorageKind string

// kubeStorageBackend stores each revision in its own Secret or ConfigMap. Listings go through
// the metadata client, so they never transfer release bodies; a body is fetched by object
// name in the revision's own namespace.
type kubeStorageBackend struct {
	client         kubernetes.Interface
	kind           kubeStorageKind
	metadataClient metadata.Interface
}

func newKubeStorageBackend(kind kubeStorageKind, client kubernetes.Interface, metadataClient metadata.Interface) *kubeStorageBackend {
	return &kubeStorageBackend{
		client:         client,
		kind:           kind,
		metadataClient: metadataClient,
	}
}

func (b *kubeStorageBackend) create(ctx context.Context, obj *storedObject) error {
	var err error

	switch b.kind {
	case kubeStorageKindSecret:
		_, err = b.client.CoreV1().Secrets(obj.Namespace).Create(ctx, newKubeSecret(obj), metav1.CreateOptions{})
	case kubeStorageKindConfigMap:
		_, err = b.client.CoreV1().ConfigMaps(obj.Namespace).Create(ctx, newKubeConfigMap(obj), metav1.CreateOptions{})
	default:
		panic(fmt.Sprintf("unexpected kube storage kind %q", b.kind))
	}

	if apierrors.IsAlreadyExists(err) {
		return ErrReleaseExists
	}

	return err
}

func (b *kubeStorageBackend) delete(ctx context.Context, namespace, key string) error {
	var err error

	switch b.kind {
	case kubeStorageKindSecret:
		err = b.client.CoreV1().Secrets(namespace).Delete(ctx, key, metav1.DeleteOptions{})
	case kubeStorageKindConfigMap:
		err = b.client.CoreV1().ConfigMaps(namespace).Delete(ctx, key, metav1.DeleteOptions{})
	default:
		panic(fmt.Sprintf("unexpected kube storage kind %q", b.kind))
	}

	if apierrors.IsNotFound(err) {
		return ErrReleaseNotFound
	}

	return err
}

func (b *kubeStorageBackend) get(ctx context.Context, namespace, key string) (*storedObject, error) {
	switch b.kind {
	case kubeStorageKindSecret:
		secret, err := b.client.CoreV1().Secrets(namespace).Get(ctx, key, metav1.GetOptions{})
		if err != nil {
			return nil, kubeStorageError(err)
		}

		return storedObjectFromKubeSecret(secret), nil
	case kubeStorageKindConfigMap:
		configMap, err := b.client.CoreV1().ConfigMaps(namespace).Get(ctx, key, metav1.GetOptions{})
		if err != nil {
			return nil, kubeStorageError(err)
		}

		return storedObjectFromKubeConfigMap(configMap), nil
	default:
		panic(fmt.Sprintf("unexpected kube storage kind %q", b.kind))
	}
}

func (b *kubeStorageBackend) gvr() schema.GroupVersionResource {
	switch b.kind {
	case kubeStorageKindSecret:
		return schema.GroupVersionResource{Version: "v1", Resource: "secrets"}
	case kubeStorageKindConfigMap:
		return schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	default:
		panic(fmt.Sprintf("unexpected kube storage kind %q", b.kind))
	}
}

func (b *kubeStorageBackend) listMetadata(ctx context.Context, namespace, releaseName string) ([]*storedObject, error) {
	selector, err := kubeStorageSelector(releaseName)
	if err != nil {
		return nil, err
	}

	var objects []*storedObject

	opts := metav1.ListOptions{LabelSelector: selector, Limit: kubeStoragePageSize}
	for {
		list, err := b.metadataClient.Resource(b.gvr()).Namespace(namespace).List(ctx, opts)
		if err != nil {
			return nil, err
		}

		for _, item := range list.Items {
			objects = append(objects, &storedObject{
				Namespace: item.Namespace,
				Key:       item.Name,
				Labels:    item.Labels,
			})
		}

		if list.Continue == "" {
			return objects, nil
		}

		opts.Continue = list.Continue
	}
}

func (b *kubeStorageBackend) listWithBodies(ctx context.Context, namespace, releaseName string, fn func(obj *storedObject) error) error {
	selector, err := kubeStorageSelector(releaseName)
	if err != nil {
		return err
	}

	opts := metav1.ListOptions{LabelSelector: selector, Limit: kubeStoragePageSize}
	for {
		var (
			objects       []*storedObject
			continueToken string
		)

		switch b.kind {
		case kubeStorageKindSecret:
			list, err := b.client.CoreV1().Secrets(namespace).List(ctx, opts)
			if err != nil {
				return err
			}

			for i := range list.Items {
				objects = append(objects, storedObjectFromKubeSecret(&list.Items[i]))
			}

			continueToken = list.Continue
		case kubeStorageKindConfigMap:
			list, err := b.client.CoreV1().ConfigMaps(namespace).List(ctx, opts)
			if err != nil {
				return err
			}

			for i := range list.Items {
				objects = append(objects, storedObjectFromKubeConfigMap(&list.Items[i]))
			}

			continueToken = list.Continue
		default:
			panic(fmt.Sprintf("unexpected kube storage kind %q", b.kind))
		}

		for i, obj := range objects {
			objects[i] = nil

			if err := fn(obj); err != nil {
				return err
			}
		}

		if continueToken == "" {
			return nil
		}

		opts.Continue = continueToken
	}
}

func (b *kubeStorageBackend) update(ctx context.Context, obj *storedObject) error {
	var err error

	switch b.kind {
	case kubeStorageKindSecret:
		_, err = b.client.CoreV1().Secrets(obj.Namespace).Update(ctx, newKubeSecret(obj), metav1.UpdateOptions{})
	case kubeStorageKindConfigMap:
		_, err = b.client.CoreV1().ConfigMaps(obj.Namespace).Update(ctx, newKubeConfigMap(obj), metav1.UpdateOptions{})
	default:
		panic(fmt.Sprintf("unexpected kube storage kind %q", b.kind))
	}

	return kubeStorageError(err)
}

func (b *kubeStorageBackend) updateLabels(ctx context.Context, namespace, key string, labels map[string]string) error {
	switch b.kind {
	case kubeStorageKindSecret:
		secret, err := b.client.CoreV1().Secrets(namespace).Get(ctx, key, metav1.GetOptions{})
		if err != nil {
			return kubeStorageError(err)
		}

		if secret.Labels == nil {
			secret.Labels = map[string]string{}
		}

		maps.Copy(secret.Labels, labels)

		_, err = b.client.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})

		return kubeStorageError(err)
	case kubeStorageKindConfigMap:
		configMap, err := b.client.CoreV1().ConfigMaps(namespace).Get(ctx, key, metav1.GetOptions{})
		if err != nil {
			return kubeStorageError(err)
		}

		if configMap.Labels == nil {
			configMap.Labels = map[string]string{}
		}

		maps.Copy(configMap.Labels, labels)

		_, err = b.client.CoreV1().ConfigMaps(namespace).Update(ctx, configMap, metav1.UpdateOptions{})

		return kubeStorageError(err)
	default:
		panic(fmt.Sprintf("unexpected kube storage kind %q", b.kind))
	}
}

func kubeStorageError(err error) error {
	if apierrors.IsNotFound(err) {
		return ErrReleaseNotFound
	}

	return err
}

func kubeStorageSelector(releaseName string) (string, error) {
	set := labels.Set{storageLabelOwner: storageOwner}

	if releaseName != "" {
		if errs := validation.IsValidLabelValue(releaseName); len(errs) != 0 {
			return "", fmt.Errorf("invalid label value %q: %s", releaseName, strings.Join(errs, "; "))
		}

		set[storageLabelName] = releaseName
	}

	return set.AsSelector().String(), nil
}

func newKubeConfigMap(obj *storedObject) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Key,
			Namespace: obj.Namespace,
			Labels:    obj.Labels,
		},
		Data: map[string]string{kubeStorageDataKey: string(obj.Body)},
	}
}

func newKubeSecret(obj *storedObject) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Key,
			Namespace: obj.Namespace,
			Labels:    obj.Labels,
		},
		Type: storageObjectType,
		Data: map[string][]byte{kubeStorageDataKey: obj.Body},
	}
}

func storedObjectFromKubeConfigMap(configMap *corev1.ConfigMap) *storedObject {
	return &storedObject{
		Namespace: configMap.Namespace,
		Key:       configMap.Name,
		Labels:    configMap.Labels,
		Body:      []byte(configMap.Data[kubeStorageDataKey]),
	}
}

func storedObjectFromKubeSecret(secret *corev1.Secret) *storedObject {
	return &storedObject{
		Namespace: secret.Namespace,
		Key:       secret.Name,
		Labels:    secret.Labels,
		Body:      secret.Data[kubeStorageDataKey],
	}
}
