package featgate

import (
	"os"

	"github.com/chanced/caps"

	"github.com/werf/nelm/internal/common"
)

var (
	FeatGateEnvVarsPrefix = caps.ToScreamingSnake(common.Brand) + "_FEAT_"
	FeatGates             = []*FeatGate{}

	// TODO(v2): always enable
	FeatGateRemoteCharts = NewFeatGate(
		"remote-charts",
		"Allow not only local, but also remote charts as an argument to cli commands. Also adds the `--chart-version` option",
	)
)

func NewFeatGate(name, help string) *FeatGate {
	fg := &FeatGate{
		Name: name,
		Help: help,
	}

	FeatGates = append(FeatGates, fg)

	return fg
}

type FeatGate struct {
	Name string
	Help string
}

func (g *FeatGate) EnvVarName() string {
	return FeatGateEnvVarsPrefix + caps.ToScreamingSnake(g.Name)
}

func (g *FeatGate) Enabled() bool {
	return os.Getenv(g.EnvVarName()) == "true"
}
