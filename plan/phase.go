package plan

type PhaseType string

const (
	PhaseTypeCreateReleaseNamespace   PhaseType = "Create release namespace"
	PhaseTypeCreateRelease                      = "Create release"
	PhaseTypeDeployPreloadedCRDs      PhaseType = "Deploy preloaded CRDs"
	PhaseTypeRunPreHelmHooks          PhaseType = "Run pre-hooks"
	PhaseTypeDeployHelmResources      PhaseType = "Deploy helm resources"
	PhaseTypeRunPostHelmHooks         PhaseType = "Run post-hooks"
	PhaseTypeCleanup                  PhaseType = "Cleanup"
	PhaseTypeSucceedRelease                     = "Succeed release"
	PhaseTypeFailRelease                        = "Fail release"
	PhaseTypeSupersedePreviousRelease           = "Supersede previous release"
	PhaseTypeDeleteReleaseNamespace   PhaseType = "Delete release namespace"
)

func NewPhase(phaseType PhaseType) *Phase {
	return &Phase{
		Type: phaseType,
	}
}

type Phase struct {
	Type       PhaseType
	Operations []Operation
}

func (p *Phase) AddOperations(operations ...Operation) *Phase {
	p.Operations = append(p.Operations, operations...)
	return p
}

func (p *Phase) ResourcesWillBeCreatedOrUpdated() bool {
	for _, operation := range p.Operations {
		if operation.ResourcesWillBeCreatedOrUpdated() {
			return true
		}
	}

	return false
}

func (p *Phase) ResourcesWillBeDeleted() bool {
	for _, operation := range p.Operations {
		if operation.ResourcesWillBeDeleted() {
			return true
		}
	}

	return false
}

func (p *Phase) ResourcesWillBeTracked() bool {
	for _, operation := range p.Operations {
		if operation.ResourcesWillBeTracked() {
			return true
		}
	}

	return false
}

func (p *Phase) ResourcesWillBeCreatedOrUpdatedOnly() bool {
	return p.ResourcesWillBeCreatedOrUpdated() && !p.ResourcesWillBeDeleted() && !p.ResourcesWillBeTracked()
}

func (p *Phase) ResourcesWillBeDeletedOnly() bool {
	return !p.ResourcesWillBeCreatedOrUpdated() && p.ResourcesWillBeDeleted() && !p.ResourcesWillBeTracked()
}

func (p *Phase) ResourcesWillBeTrackedOnly() bool {
	return !p.ResourcesWillBeCreatedOrUpdated() && !p.ResourcesWillBeDeleted() && p.ResourcesWillBeTracked()
}
