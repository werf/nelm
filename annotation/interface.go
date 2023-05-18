package annotation

type Annotationer interface {
	Validate() error

	Key() string
	Value() string
	String() string
}
