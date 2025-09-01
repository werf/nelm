package operation

const (
	OperationTypeNoop    = "noop"
	OperationVersionNoop = 1
)

var _ OperationConfig = (*OperationConfigNoop)(nil)

type OperationConfigNoop struct {
	OpID string
}

func (c *OperationConfigNoop) ID() string {
	return c.OpID
}

func (c *OperationConfigNoop) IDHuman() string {
	return c.OpID
}
