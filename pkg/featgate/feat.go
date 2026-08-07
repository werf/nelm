package featgate

import (
	"os"

	"github.com/chanced/caps"
	"github.com/samber/lo"

	"github.com/werf/nelm/pkg/common"
)

var (
	FeatGateEnvVarsPrefix = caps.ToScreamingSnake(common.Brand) + "_FEAT_"
	// Contains all defined feature gates.
	FeatGates                   = []*FeatGate{}
	FeatGatePeriodicStackTraces = NewFeatGate(
		"periodic-stack-traces",
		`Print stack traces periodically to help with debugging deadlocks and other issues`,
	)
	FeatGateTypescript = NewFeatGate(
		"typescript",
		`Enable TypeScript chart rendering from ts/ directory`,
	)
	FeatGateAdoptDeckhouseControllerFields = NewFeatGate(
		"adopt-deckhouse-controller-fields",
		`Adopt managed fields owned by the legacy "deckhouse-controller" field manager (the pre-nelm Helm 3 engine). Unsafe if any resource still has hook-owned "deckhouse-controller" entries`,
	)
	FeatGateCaseInsensitiveConditionTracking = NewFeatGate(
		"case-insensitive-condition-tracking",
		`Match custom resource status condition types case-insensitively when detecting readiness (e.g. "Ready" in addition to "ready")`,
	)
)

// A feature gate, which enabled/disables a specific feature. Can be toggled via an env var or
// programmatically.
type FeatGate struct {
	Help string
	Name string

	forceEnabled *bool
}

func NewFeatGate(name, help string) *FeatGate {
	fg := &FeatGate{
		Help: help,
		Name: name,
	}

	FeatGates = append(FeatGates, fg)

	return fg
}

func (g *FeatGate) Default() bool {
	return false
}

func (g *FeatGate) Disable() {
	g.forceEnabled = lo.ToPtr(false)
}

func (g *FeatGate) Enable() {
	g.forceEnabled = lo.ToPtr(true)
}

func (g *FeatGate) Enabled() bool {
	if g.forceEnabled != nil {
		return *g.forceEnabled
	}

	return os.Getenv(g.EnvVarName()) == "true"
}

func (g *FeatGate) EnvVarName() string {
	return FeatGateEnvVarsPrefix + caps.ToScreamingSnake(g.Name)
}
