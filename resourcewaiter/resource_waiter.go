package resourcewaiter

import (
	"context"
	"fmt"
	"io"
	"time"

	"helm.sh/helm/v3/pkg/werf/resource"
	"helm.sh/helm/v3/pkg/werf/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
)

func NewResourceWaiter(dynamicClient dynamic.Interface, mapper meta.ResettableRESTMapper) *ResourceWaiter {
	return &ResourceWaiter{
		dynamicClient: dynamicClient,
		mapper:        mapper,
	}
}

type ResourceWaiter struct {
	dynamicClient dynamic.Interface
	mapper        meta.ResettableRESTMapper
}

func (w *ResourceWaiter) WaitDeletion(ctx context.Context, options WaitDeletionOptions, refs ...resource.Referencer) error {
	for _, ref := range refs {
		gvr, namespaced, err := util.ConvertGVKtoGVR(ref.GroupVersionKind(), w.mapper)
		if err != nil {
			return fmt.Errorf("error converting kind %q to resource: %w", ref.GroupVersionKind(), err)
		}

		var namespace string
		if ref.Namespace() != "" {
			namespace = ref.Namespace()
		} else {
			namespace = options.FallbackNamespace
		}

		var clientResource dynamic.ResourceInterface
		if namespaced {
			clientResource = w.dynamicClient.Resource(gvr).Namespace(namespace)
		} else {
			clientResource = w.dynamicClient.Resource(gvr)
		}

		var timeout time.Duration
		if options.Timeout > 0 {
			timeout = time.Duration(options.Timeout) * time.Second
		} else {
			timeout = 5 * time.Minute
		}

		if err := wait.PollImmediate(700*time.Millisecond, timeout, func() (bool, error) {
			_, err := clientResource.Get(ctx, ref.Name(), metav1.GetOptions{})
			if err != nil {
				if apierrors.IsResourceExpired(err) || apierrors.IsGone(err) || err == io.EOF || err == io.ErrUnexpectedEOF {
					return false, nil
				}

				if apierrors.IsNotFound(err) {
					return true, nil
				}

				return false, fmt.Errorf("error getting resource: %w", err)
			}

			return false, nil
		}); err != nil {
			return fmt.Errorf("error polling for resource: %w", err)
		}
	}

	return nil
}

// func (w *ResourceWaiter) WaitDeletion(ctx context.Context, options WaitDeletionOptions, refs ...resource.Referencer) error {
// 	for _, ref := range refs {
// 		gvr, namespaced, err := util.ConvertGVKtoGVR(ref.GroupVersionKind(), w.mapper)
// 		if err != nil {
// 			return fmt.Errorf("error converting kind %q to resource: %w", ref.GroupVersionKind(), err)
// 		}
//
// 		var namespace string
// 		if ref.Namespace() != "" {
// 			namespace = ref.Namespace()
// 		} else {
// 			namespace = options.FallbackNamespace
// 		}
//
// 		var clientResource dynamic.ResourceInterface
// 		if namespaced {
// 			clientResource = w.dynamicClient.Resource(gvr).Namespace(namespace)
// 		} else {
// 			clientResource = w.dynamicClient.Resource(gvr)
// 		}
//
// 		var (
// 			informerCtx       context.Context
// 			informerCtxCancel context.CancelFunc
// 		)
// 		if options.Timeout > 0 {
// 			timeout := time.Duration(options.Timeout) * time.Second
// 			informerCtx, informerCtxCancel = context.WithTimeout(ctx, timeout)
// 		} else {
// 			informerCtx, informerCtxCancel = context.WithCancel(ctx)
// 		}
//
// 		informer := cache.NewSharedIndexInformer(
// 			&cache.ListWatch{
// 				ListFunc: func(_ metav1.ListOptions) (runtime.Object, error) {
// 					unstruct, err := clientResource.Get(informerCtx, ref.Name(), metav1.GetOptions{})
// 					if apierrors.IsNotFound(err) {
// 						informerCtxCancel()
// 						return unstruct, nil
// 					}
// 					return unstruct, err
// 				},
// 				WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
// 					opts.Kind = ref.GroupVersionKind().Kind
// 					opts.APIVersion = ref.GroupVersionKind().GroupVersion().String()
// 					opts.FieldSelector = fields.Set{"metadata.name": ref.Name()}.AsSelector().String()
// 					return clientResource.Watch(informerCtx, opts)
// 				},
// 			},
// 			&unstructured.Unstructured{},
// 			0,
// 			cache.Indexers{},
// 		)
//
// 		informer.AddEventHandler(
// 			cache.ResourceEventHandlerFuncs{
// 				DeleteFunc: func(obj interface{}) {
// 					informerCtxCancel()
// 				},
// 			},
// 		)
//
// 		var (
// 			fatalWatchErr     error
// 			fatalWatchErrOnce sync.Once
// 		)
// 		informer.SetWatchErrorHandler(func(r *cache.Reflector, err error) {
// 			if apierrors.IsResourceExpired(err) || apierrors.IsGone(err) || err == io.EOF || err == io.ErrUnexpectedEOF {
// 				return
// 			}
//
// 			fatalWatchErrOnce.Do(func() {
// 				fatalWatchErr = err
// 			})
// 			informerCtxCancel()
// 		})
//
// 		informer.Run(informerCtx.Done())
//
// 		if fatalWatchErr != nil {
// 			return fmt.Errorf("fatal watch error: %w", fatalWatchErr)
// 		}
// 	}
//
// 	return nil
// }

type WaitDeletionOptions struct {
	FallbackNamespace string
	Timeout           int
}
