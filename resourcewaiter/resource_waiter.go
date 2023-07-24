package resourcewaiter

import (
	"context"
	"fmt"
	"io"
	"time"

	"helm.sh/helm/v3/pkg/werf/log"
	"helm.sh/helm/v3/pkg/werf/util"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
)

const DefaultTrackTimeout = 5 * time.Minute

func NewResourceWaiter(dynamicClient dynamic.Interface, mapper meta.ResettableRESTMapper, opts NewResourceWaiterOptions) *ResourceWaiter {
	var logger log.Logger
	if opts.Logger != nil {
		logger = opts.Logger
	} else {
		logger = log.NewNullLogger()
	}

	var defaultTrackTimeout time.Duration
	if opts.DefaultTrackTimeout > 0 {
		defaultTrackTimeout = opts.DefaultTrackTimeout
	} else {
		defaultTrackTimeout = DefaultTrackTimeout
	}

	return &ResourceWaiter{
		dynamicClient: dynamicClient,
		mapper:        mapper,

		defaultTrackTimeout: defaultTrackTimeout,

		logger: logger,
	}
}

type NewResourceWaiterOptions struct {
	Logger              log.Logger
	DefaultTrackTimeout time.Duration
}

type ResourceWaiter struct {
	dynamicClient dynamic.Interface
	mapper        meta.ResettableRESTMapper

	defaultTrackTimeout time.Duration

	logger log.Logger
}

func (w *ResourceWaiter) WaitDeletion(ctx context.Context, res Waitable, opts WaitDeletionOptions) error {
	gvr, namespaced, err := util.ConvertGVKtoGVR(res.GroupVersionKind(), w.mapper)
	if err != nil {
		return fmt.Errorf("error mapping GroupVersionKind %q to GroupVersionResource: %w", res.GroupVersionKind(), err)
	}

	var (
		clientResource dynamic.ResourceInterface
		namespace      string
	)
	if namespaced {
		if res.Namespace() != "" {
			namespace = res.Namespace()
		} else if opts.FallbackNamespace != "" {
			namespace = opts.FallbackNamespace
		} else {
			namespace = v1.NamespaceDefault
		}
		clientResource = w.dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		clientResource = w.dynamicClient.Resource(gvr)
	}

	var timeout time.Duration
	if opts.Timeout > 0 {
		timeout = opts.Timeout
	} else {
		timeout = w.defaultTrackTimeout
	}

	w.logger.Debug("Polling for resource %q ...", res.String())
	if err := wait.PollImmediate(700*time.Millisecond, timeout, func() (bool, error) {
		_, err := clientResource.Get(ctx, res.Name(), metav1.GetOptions{})
		if err != nil {
			if apierrors.IsResourceExpired(err) || apierrors.IsGone(err) || err == io.EOF || err == io.ErrUnexpectedEOF {
				return false, nil
			}

			if apierrors.IsNotFound(err) {
				return true, nil
			}

			return false, fmt.Errorf("error getting resource %q: %w", res.String(), err)
		}

		return false, nil
	}); err != nil {
		return fmt.Errorf("error polling resource %q: %w", res.String(), err)
	}
	w.logger.Debug("Polled for resource %q", res.String())

	return nil
}

type WaitDeletionOptions struct {
	FallbackNamespace string
	Timeout           time.Duration
}

type Waitable interface {
	Name() string
	Namespace() string
	GroupVersionKind() schema.GroupVersionKind
	String() string
	// FIXME(ilya-lesikov):
	// HumanID() string
}
