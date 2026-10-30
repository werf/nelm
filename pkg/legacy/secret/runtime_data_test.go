package secret

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatSecretValuePath(t *testing.T) {
	tests := []struct {
		name     string
		path     []interface{}
		expected string
	}{
		{name: "nested keys", path: []interface{}{"a", "b_c", "d-e"}, expected: "a.b_c.d-e"},
		{name: "sequence indexes", path: []interface{}{"list", 0, "port", 1}, expected: "list[0].port[1]"},
		{name: "root sequence", path: []interface{}{0, "a"}, expected: "[0].a"},
		{name: "dotted key", path: []interface{}{"a.b"}, expected: `["a.b"]`},
		{name: "dotted key after a key", path: []interface{}{"a", "b.c", "d"}, expected: `a["b.c"].d`},
		{name: "bracketed key", path: []interface{}{"a", "[0]"}, expected: `a["[0]"]`},
		{name: "empty key", path: []interface{}{"a", ""}, expected: `a[""]`},
		{name: "multiline key", path: []interface{}{"a\nb"}, expected: `["a\nb"]`},
		{name: "control character and quote", path: []interface{}{"a\x1b\"b"}, expected: `["a\x1b\"b"]`},
		{name: "key starting with a digit", path: []interface{}{"1a"}, expected: `["1a"]`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, formatSecretValuePath(test.path))
		})
	}
}
