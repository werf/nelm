package flag

const (
	// FIXME(ilya-lesikov): group all env vars and display this in usage
	GroupIDAnnotationName    = "group-id"
	GroupTitleAnnotationName = "group-title"
)

type Group struct {
	ID    string
	Title string
}
