package featgate

import (
	"os"

	"github.com/chanced/caps"

	"github.com/werf/nelm/pkg/common"
)

var (
	FeatGateEnvVarsPrefix = caps.ToScreamingSnake(common.Brand) + "_FEAT_"
	FeatGates             = []*FeatGate{}

	FeatGateRemoteCharts = NewFeatGate(
		"remote-charts",
		`Allow not only local, but also remote charts as an argument to cli commands. Also adds the "--chart-version" option`,
	)

	FeatGateNativeReleaseList = NewFeatGate(
		"native-release-list",
		`Use the native "release list" command instead of "helm list" exposed as "release list"`,
	)

	FeatGatePeriodicStackTraces = NewFeatGate(
		"periodic-stack-traces",
		`Print stack traces periodically to help with debugging deadlocks and other issues`,
	)

	FeatGateNativeReleaseUninstall = NewFeatGate(
		"native-release-uninstall",
		`Use the new "release uninstall" command implementation (not fully backwards compatible)`,
	)

	FeatGateFieldSensitive = NewFeatGate(
		"field-sensitive",
		`Enable JSONPath-based selective sensitive field redaction`,
	)

	FeatGatePreviewV2 = NewFeatGate(
		"preview-v2",
		`Activate all feature gates that will be enabled by default in Nelm v2`,
	)
)

type FeatGate struct {
	Name string
	Help string
}

func NewFeatGate(name, help string) *FeatGate {
	fg := &FeatGate{
		Name: name,
		Help: help,
	}

	FeatGates = append(FeatGates, fg)

	return fg
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
