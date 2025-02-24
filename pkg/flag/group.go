package flag

const (
	GroupIDAnnotationName       = "group-id"
	GroupTitleAnnotationName    = "group-title"
	GroupPriorityAnnotationName = "group-priority"
)

func NewGroup(id, title string, priority int) *Group {
	return &Group{
		ID:       id,
		Title:    title,
		Priority: priority,
	}
}

type Group struct {
	ID       string
	Title    string
	Priority int
}
