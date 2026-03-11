package phases

import (
	"github.com/werf/nelm/internal/helm/pkg/phases/stages"
)

type NoExternalDepsGenerator struct{}

func (g *NoExternalDepsGenerator) Generate(_ stages.SortedStageList) error {
	return nil
}
