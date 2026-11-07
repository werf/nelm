package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLoopbackHost(t *testing.T) {
	tests := []struct {
		host     string
		expected bool
	}{
		{host: "localhost", expected: true},
		{host: "localhost:32768", expected: true},
		{host: "127.0.0.1:5000", expected: true},
		{host: "127.1.2.3", expected: true},
		{host: "[::1]:5000", expected: true},
		{host: "::1", expected: true},
		{host: "registry.example.com", expected: false},
		{host: "registry.example.com:5000", expected: false},
		{host: "10.0.0.1:5000", expected: false},
		{host: "localhost.example.com", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsLoopbackHost(tt.host))
		})
	}
}
