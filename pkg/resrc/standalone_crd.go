package resrc

import (
	"fmt"
	"strings"

	"helm.sh/helm/v3/pkg/werf/resrcid"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
)

const TypeStandaloneCRD Type = "standalone-crd"

func NewStandaloneCRD(unstruct *unstructured.Unstructured, opts StandaloneCRDOptions) *StandaloneCRD {
	resID := resrcid.NewResourceIDFromUnstruct(unstruct, resrcid.ResourceIDOptions{
		FilePath:         opts.FilePath,
		DefaultNamespace: opts.DefaultNamespace,
		Mapper:           opts.Mapper,
	})

	return &StandaloneCRD{
		ResourceID: resID,
		unstruct:   unstruct,
		mapper:     opts.Mapper,
	}
}

type StandaloneCRDOptions struct {
	FilePath         string
	DefaultNamespace string
	Mapper           meta.ResettableRESTMapper
}

func NewStandaloneCRDFromManifest(manifest string, opts StandaloneCRDFromManifestOptions) (*StandaloneCRD, error) {
	var filepath string
	if opts.FilePath != "" {
		filepath = opts.FilePath
	} else if strings.HasPrefix(manifest, "# Source: ") {
		firstLine := strings.TrimSpace(strings.Split(manifest, "\n")[0])
		filepath = strings.TrimPrefix(firstLine, "# Source: ")
	}

	obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(manifest), nil, &unstructured.Unstructured{})
	if err != nil {
		return nil, fmt.Errorf("error decoding CRD from file %q: %w", filepath, err)
	}

	unstructObj := obj.(*unstructured.Unstructured)

	crd := NewStandaloneCRD(unstructObj, StandaloneCRDOptions{
		FilePath:         filepath,
		DefaultNamespace: opts.DefaultNamespace,
		Mapper:           opts.Mapper,
	})

	return crd, nil
}

type StandaloneCRDFromManifestOptions struct {
	FilePath         string
	DefaultNamespace string
	Mapper           meta.ResettableRESTMapper
}

type StandaloneCRD struct {
	*resrcid.ResourceID

	unstruct *unstructured.Unstructured
	mapper   meta.ResettableRESTMapper
}

func (r *StandaloneCRD) Validate() error {
	return nil
}

func (r *StandaloneCRD) Unstructured() *unstructured.Unstructured {
	return r.unstruct
}

func (r *StandaloneCRD) ManageableBy() ManageableBy {
	return ManageableByAnyone
}

func (r *StandaloneCRD) Type() Type {
	return TypeStandaloneCRD
}
