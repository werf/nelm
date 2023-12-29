package log

var Default Logger = DefaultLogboek
var DefaultLogboek = NewLogboekLogger()
var DefaultNull = NewNullLogger()
