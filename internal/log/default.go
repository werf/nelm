package log

var (
	Default        Logger = DefaultLogboek
	DefaultLogboek        = NewLogboekLogger()
	DefaultNull           = NewNullLogger()
)
