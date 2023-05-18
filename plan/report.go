package plan

import "helm.sh/helm/v3/pkg/werf/resource"

func NewReport() *Report {
	return &Report{}
}

type Report struct {
	Created []struct {
		Target resource.Referencer
		Result *resource.GenericResource
	}
	Recreated []struct {
		Target resource.Referencer
		Result *resource.GenericResource
	}
	Updated []struct {
		Target resource.Referencer
		Result *resource.GenericResource
	}
	Deleted []resource.Referencer
}
