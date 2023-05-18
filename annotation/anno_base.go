package annotation

func newBaseAnnotation(key, value string) *baseAnnotation {
	return &baseAnnotation{
		key:   key,
		value: value,
	}
}

type baseAnnotation struct {
	key   string
	value string
}

func (a *baseAnnotation) Key() string {
	return a.key
}

func (a *baseAnnotation) Value() string {
	return a.value
}

func (a *baseAnnotation) String() string {
	return a.Key() + "=" + a.Value()
}
