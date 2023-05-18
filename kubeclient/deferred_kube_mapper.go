package kubeclient

import (
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/api/meta"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func NewDeferredKubeMapper(factory cmdutil.Factory) *DeferredKubeMapper {
	return &DeferredKubeMapper{
		factory: factory,
	}
}

type DeferredKubeMapper struct {
	meta.ResettableRESTMapper
	factory cmdutil.Factory
}

func (m *DeferredKubeMapper) Init() error {
	mapper, err := m.factory.ToRESTMapper()
	if err != nil {
		return fmt.Errorf("error getting REST mapper: %w", err)
	}

	m.ResettableRESTMapper = reflect.ValueOf(mapper).Interface().(meta.ResettableRESTMapper)

	return nil
}
