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
		`Allow not only local, but also remote charts as an argument to cli commands. Also adds the "--chart-version" option`,
	)

	// TODO(v2): always enable
	FeatGateNativeReleaseList = NewFeatGate(
		"native-release-list",
		`Use the native "release list" command instead of "helm list" exposed as "release list"`,
	)

	FeatGatePeriodicStackTraces = NewFeatGate(
		"periodic-stack-traces",
		`Print stack traces periodically to help with debugging deadlocks and other issues`,
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

func (g *FeatGate) Default() bool {
	return false
}

func (g *FeatGate) Enabled() bool {
	return os.Getenv(g.EnvVarName()) == "true"
}
