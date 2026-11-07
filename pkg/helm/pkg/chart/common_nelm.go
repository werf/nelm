package chart

import (
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
)

func (r *v2Accessor) WerfDataFiles() []*common.File {
	return r.chrt.WerfDataFiles
}

func (r *v3Accessor) WerfDataFiles() []*common.File {
	return r.chrt.WerfDataFiles
}
