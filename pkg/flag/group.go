package flag

const (
	GroupIDAnnotationName    = "group-id"
	GroupTitleAnnotationName = "group-title"
)

type Group struct {
	ID    string
	Title string
}
