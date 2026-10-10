package release

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
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

	if err != nil {
		return fmt.Errorf("create %s: %w", b.kind, err)
	}

	return nil
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

	if err != nil {
		return fmt.Errorf("delete %s: %w", b.kind, err)
	}

	return nil
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

func (b *kubeStorageBackend) listBodies(ctx context.Context, namespace, selector string, fn func(obj *storedObject) error) error {
	opts := metav1.ListOptions{LabelSelector: selector, Limit: kubeStoragePageSize}
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("list %s: %w", b.kind, err)
		}

		var continueToken string

		switch b.kind {
		case kubeStorageKindSecret:
			list, err := b.client.CoreV1().Secrets(namespace).List(ctx, opts)
			if err != nil {
				return fmt.Errorf("list secrets: %w", err)
			}

			for i := range list.Items {
				if err := ctx.Err(); err != nil {
					return fmt.Errorf("list %s: %w", b.kind, err)
				}

				obj := storedObjectFromKubeSecret(&list.Items[i])
				list.Items[i] = corev1.Secret{}

				if err := fn(obj); err != nil {
					return err
				}
			}

			continueToken = list.Continue
		case kubeStorageKindConfigMap:
			list, err := b.client.CoreV1().ConfigMaps(namespace).List(ctx, opts)
			if err != nil {
				return fmt.Errorf("list configmaps: %w", err)
			}

			for i := range list.Items {
				if err := ctx.Err(); err != nil {
					return fmt.Errorf("list %s: %w", b.kind, err)
				}

				obj := storedObjectFromKubeConfigMap(&list.Items[i])
				list.Items[i] = corev1.ConfigMap{}

				if err := fn(obj); err != nil {
					return err
				}
			}

			continueToken = list.Continue
		default:
			panic(fmt.Sprintf("unexpected kube storage kind %q", b.kind))
		}

		if continueToken == "" {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("list %s: %w", b.kind, err)
			}

			return nil
		}

		opts.Continue = continueToken
	}
}

func (b *kubeStorageBackend) listMetadata(ctx context.Context, namespace, releaseName string) ([]*storedObject, error) {
	selector, err := kubeStorageSelector(releaseName, nil)
	if err != nil {
		return nil, err
	}

	var objects []*storedObject
	if err := b.listMetadataPages(ctx, namespace, selector, func(obj *storedObject) error {
		objects = append(objects, obj)

		return nil
	}); err != nil {
		return nil, err
	}

	return objects, nil
}

func (b *kubeStorageBackend) listMetadataPages(ctx context.Context, namespace, selector string, fn func(obj *storedObject) error) error {
	opts := metav1.ListOptions{LabelSelector: selector, Limit: kubeStoragePageSize}
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("list %s metadata: %w", b.kind, err)
		}

		list, err := b.metadataClient.Resource(b.gvr()).Namespace(namespace).List(ctx, opts)
		if err != nil {
			return fmt.Errorf("list %s metadata: %w", b.kind, err)
		}

		for _, item := range list.Items {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("list %s metadata: %w", b.kind, err)
			}

			if err := fn(&storedObject{
				Namespace: item.Namespace,
				Key:       item.Name,
				Labels:    item.Labels,
			}); err != nil {
				return err
			}
		}

		if list.Continue == "" {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("list %s metadata: %w", b.kind, err)
			}

			return nil
		}

		opts.Continue = list.Continue
	}
}

func (b *kubeStorageBackend) listWithBodies(ctx context.Context, namespace, releaseName string, versions []int, fn func(obj *storedObject) error) error {
	selector, err := kubeStorageSelector(releaseName, versions)
	if err != nil {
		return err
	}

	return b.listBodies(ctx, namespace, selector, fn)
}

func (b *kubeStorageBackend) scanLatestCandidates(ctx context.Context, namespace string, selector labels.Selector, withBodies bool, fn func(obj *storedObject) error) error {
	if selector == nil {
		selector = labels.Everything()
	}

	requirements, selectable := selector.Requirements()
	if !selectable {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("list %s: %w", b.kind, err)
		}

		return nil
	}

	combined := labels.Set{storageLabelOwner: storageOwner}.AsSelector().Add(requirements...)

	if withBodies {
		return b.listBodies(ctx, namespace, combined.String(), fn)
	}

	return b.listMetadataPages(ctx, namespace, combined.String(), fn)
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

func kubeStorageSelector(releaseName string, versions []int) (string, error) {
	set := labels.Set{storageLabelOwner: storageOwner}

	if releaseName != "" {
		if errs := validation.IsValidLabelValue(releaseName); len(errs) != 0 {
			return "", fmt.Errorf("invalid label value %q: %s", releaseName, strings.Join(errs, "; "))
		}

		set[storageLabelName] = releaseName
	}

	selector := set.AsSelector()

	if versions != nil {
		requirement, err := labels.NewRequirement(storageLabelVersion, selection.In, lo.Map(versions, func(version, _ int) string { return strconv.Itoa(version) }))
		if err != nil {
			return "", fmt.Errorf("build version label requirement: %w", err)
		}

		selector = selector.Add(*requirement)
	}

	return selector.String(), nil
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
