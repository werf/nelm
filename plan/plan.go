package plan

type PlanType string

const (
	PlanTypeDeploy               PlanType = "Deploy"
	PlanTypeFinalizeFailedDeploy PlanType = "FinalizeFailedDeploy"
)

func NewPlan(planType PlanType) *Plan {
	return &Plan{
		Type: planType,
	}
}

type Plan struct {
	Type   PlanType
	Phases []*Phase
}

func (p *Plan) AddPhases(phases ...*Phase) *Plan {
	p.Phases = append(p.Phases, phases...)
	return p
}

func (p *Plan) Empty() bool {
	return len(p.Phases) == 0
}
