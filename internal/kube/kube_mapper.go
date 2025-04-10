package kube

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/restmapper"

	"github.com/werf/nelm/internal/log"
)

func NewKubeMapper(ctx context.Context, discoveryClient discovery.CachedDiscoveryInterface) meta.RESTMapper {
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)

	expander := restmapper.NewShortcutExpander(mapper, discoveryClient, func(msg string) {
		log.Default.Warn(ctx, msg)
	})

	return expander
}
