package release

import "fmt"

type Phase string

const (
	PhaseInit      Phase = "init"
	PhaseHooksPre  Phase = "hooks-pre"
	PhaseRollout   Phase = "rollout"
	PhaseUninstall Phase = "uninstall"
	PhaseHooksPost Phase = "hooks-post"
)

// May return empty string.
func PhaseFromHookEvent(hookEvent HookEvent) Phase {
	var phase Phase
	switch hookEvent {
	case HookPreInstall, HookPreDelete, HookPreUpgrade, HookPreRollback:
		phase = PhaseHooksPre
	case HookPostInstall, HookPostDelete, HookPostUpgrade, HookPostRollback:
		phase = PhaseHooksPost
	case HookTest:
	default:
		panic(fmt.Sprintf("unexpected HookEvent: %s", hookEvent.String()))
	}

	return phase
}

func SetInitPhaseStageInfo(rel *Release) *Release {
	lastPhase := PhaseInit
	lastStage := 0
	rel.Info.LastPhase = &lastPhase
	rel.Info.LastStage = &lastStage

	return rel
}

func SetHookPhaseStageInfo(rel *Release, hookIndex int, hook HookEvent) *Release {
	lastPhase := PhaseFromHookEvent(hook)
	rel.Info.LastPhase = &lastPhase
	rel.Info.LastStage = &hookIndex

	return rel
}

func SetRolloutPhaseStageInfo(rel *Release, stageIndex int) *Release {
	lastPhase := PhaseRollout
	rel.Info.LastPhase = &lastPhase
	rel.Info.LastStage = &stageIndex

	return rel
}

func SetUninstallPhaseStageInfo(rel *Release) *Release {
	lastPhase := PhaseUninstall
	rel.Info.LastPhase = &lastPhase
	rel.Info.LastStage = nil

	return rel
}
