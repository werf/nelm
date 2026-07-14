//go:build ai_tests

package action

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAI_ParseLocalLookupResourcesDuplicateAfterExpansion(t *testing.T) {
	path := writeLocalLookupFile(t, `
apiVersion: v1
kind: Pod
metadata:
  name: pod1
  namespace: default
---
apiVersion: v1
kind: List
items:
  - apiVersion: v1
    kind: Pod
    metadata:
      name: pod1
      namespace: default
`)

	_, err := parseLocalLookupResources([]string{path})
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate resource")
	require.Contains(t, err.Error(), "item 0")
}

func TestAI_ParseLocalLookupResourcesListExpansion(t *testing.T) {
	path := writeLocalLookupFile(t, `
apiVersion: v1
kind: List
items:
  - apiVersion: v1
    kind: Pod
    metadata:
      name: pod1
      namespace: default
  - apiVersion: v1
    kind: Pod
    metadata:
      name: pod2
      namespace: default
`)

	resources, err := parseLocalLookupResources([]string{path})
	require.NoError(t, err)
	require.Len(t, resources, 2)
	require.Equal(t, "pod1", resources[0].GetName())
	require.Equal(t, "pod2", resources[1].GetName())
	for _, r := range resources {
		require.Equal(t, "Pod", r.GetKind())
	}
}

func TestAI_ParseLocalLookupResourcesListItemMissingAPIVersion(t *testing.T) {
	path := writeLocalLookupFile(t, `
apiVersion: v1
kind: List
items:
  - kind: Pod
    metadata:
      name: pod1
      namespace: default
`)

	_, err := parseLocalLookupResources([]string{path})
	require.Error(t, err)
	require.Contains(t, err.Error(), "apiVersion is missing")
	require.Contains(t, err.Error(), "item 0")
}

func TestAI_ParseLocalLookupResourcesMultiDoc(t *testing.T) {
	path := writeLocalLookupFile(t, `
apiVersion: v1
kind: Pod
metadata:
  name: pod1
  namespace: default
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm1
  namespace: default
`)

	resources, err := parseLocalLookupResources([]string{path})
	require.NoError(t, err)
	require.Len(t, resources, 2)
	require.Equal(t, "Pod", resources[0].GetKind())
	require.Equal(t, "ConfigMap", resources[1].GetKind())
}

func TestAI_ParseLocalLookupResourcesTopLevelDuplicate(t *testing.T) {
	path := writeLocalLookupFile(t, `
apiVersion: v1
kind: Pod
metadata:
  name: pod1
  namespace: default
---
apiVersion: v1
kind: Pod
metadata:
  name: pod1
  namespace: default
`)

	_, err := parseLocalLookupResources([]string{path})
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate resource")
}

func TestAI_ParseLocalLookupResourcesTopLevelMissingAPIVersion(t *testing.T) {
	path := writeLocalLookupFile(t, `
kind: Pod
metadata:
  name: pod1
  namespace: default
`)

	_, err := parseLocalLookupResources([]string{path})
	require.Error(t, err)
	require.Contains(t, err.Error(), "apiVersion is missing")
}

func TestAI_ParseLocalLookupResourcesTypedListExpansion(t *testing.T) {
	path := writeLocalLookupFile(t, `
apiVersion: v1
kind: PodList
items:
  - apiVersion: v1
    kind: Pod
    metadata:
      name: pod1
      namespace: default
`)

	resources, err := parseLocalLookupResources([]string{path})
	require.NoError(t, err)
	require.Len(t, resources, 1)
	require.Equal(t, "Pod", resources[0].GetKind())
	require.Equal(t, "pod1", resources[0].GetName())
}

func writeLocalLookupFile(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "resources.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	return path
}
