package registry

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/registry/remote/auth"
)

func TestClientOptBasicAuthHosts(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir())

	basic := auth.Credential{Username: "user", Password: "secret"}

	tests := []struct {
		name     string
		hosts    []string
		hostport string
		expected auth.Credential
	}{
		{name: "listed host", hosts: []string{"registry.example.com"}, hostport: "registry.example.com", expected: basic},
		{name: "listed host with port", hosts: []string{"localhost:5000"}, hostport: "localhost:5000", expected: basic},
		{name: "docker.io", hosts: []string{"docker.io"}, hostport: "registry-1.docker.io", expected: basic},
		{name: "index.docker.io", hosts: []string{"index.docker.io"}, hostport: "registry-1.docker.io", expected: basic},
		{name: "other host", hosts: []string{"registry.example.com"}, hostport: "other.example.com", expected: auth.EmptyCredential},
		{name: "no hosts", hosts: []string{}, hostport: "registry.example.com", expected: auth.EmptyCredential},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(
				ClientOptBasicAuth("user", "secret"),
				ClientOptBasicAuthHosts(tt.hosts...),
				ClientOptCredentialsFile(filepath.Join(t.TempDir(), "config.json")),
			)
			require.NoError(t, err)

			credential, err := client.authorizer.Credential(context.Background(), tt.hostport)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, credential)
		})
	}
}
