package kube

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ResourcesWaiter interface {
	Wait(ctx context.Context, resources ResourceList, timeout time.Duration) error
	WatchUntilReady(ctx context.Context, resources ResourceList, timeout time.Duration) error
	WaitUntilDeleted(ctx context.Context, specs []*ResourcesWaiterDeleteResourceSpec, timeout time.Duration) error
}

type ResourcesWaiterDeleteResourceSpec struct {
	ResourceName         string
	Namespace            string
	GroupVersionResource schema.GroupVersionResource
}
