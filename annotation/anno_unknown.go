package annotation

var _ Annotationer = (*AnnotationUnknown)(nil)

func NewAnnotationUnknown(key, value string) *AnnotationUnknown {
	return &AnnotationUnknown{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationUnknown struct{ *baseAnnotation }

func (a *AnnotationUnknown) Validate() error {
	// 	TODO(ilya-lesikov): generic validate key and value
	return nil
}
