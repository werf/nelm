package chartyaml

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testChartYAML = `# The chart.
apiVersion: v2   # keep me
name: mychart
# version comment
version: "1.0.0"
description: >-
  folded
  text
dependencies:
  - name: dep
    version: 1.x
    repository: "https://example.com"

keywords: [a, b]
`

func TestEdit(t *testing.T) {
	images := "- name: backend\n  image: registry/app:backend\n"

	tests := []struct {
		name     string
		data     string
		opts     EditOptions
		expected string
	}{
		{
			name:     "nothing to change",
			data:     "name:   mychart\n",
			opts:     EditOptions{},
			expected: "name:   mychart\n",
		},
		{
			name: "fields",
			data: testChartYAML,
			opts: EditOptions{Name: "renamed", Version: "1.2.3", AppVersion: "2.0"},
			expected: `# The chart.
apiVersion: v2 # keep me
name: renamed
# version comment
version: 1.2.3
description: >-
  folded
  text
dependencies:
  - name: dep
    version: 1.x
    repository: "https://example.com"

keywords: [a, b]
appVersion: "2.0"
`,
		},
		{
			name: "add annotations",
			data: "name: mychart # c\nversion: 1.0.0\n",
			opts: EditOptions{SetAnnotations: map[string]string{ImagesAnnotation: images}},
			expected: `name: mychart # c
version: 1.0.0
annotations:
  helm.sh/images: |
    - name: backend
      image: registry/app:backend
`,
		},
		{
			name: "add to existing annotations",
			data: "name: mychart\nannotations:\n    # team comment\n    team: payments\nversion: 1.0.0\n",
			opts: EditOptions{SetAnnotations: map[string]string{ImagesAnnotation: images}},
			expected: `name: mychart
annotations:
    # team comment
    team: payments
    helm.sh/images: |
      - name: backend
        image: registry/app:backend
version: 1.0.0
`,
		},
		{
			name: "replace annotation",
			data: "name: mychart\nannotations:\n  helm.sh/images: |\n    - name: old\n      image: old\n  team: payments\nversion: 1.0.0\n",
			opts: EditOptions{SetAnnotations: map[string]string{ImagesAnnotation: images}},
			expected: `name: mychart
annotations:
  helm.sh/images: |
    - name: backend
      image: registry/app:backend
  team: payments
version: 1.0.0
`,
		},
		{
			name:     "delete the only annotation",
			data:     "name: mychart\nannotations:\n  helm.sh/images: |\n    - name: old\n      image: old\nversion: 1.0.0\n",
			opts:     EditOptions{DeleteAnnotations: []string{ImagesAnnotation}},
			expected: "name: mychart\nversion: 1.0.0\n",
		},
		{
			name: "flow annotations",
			data: "name: mychart\nannotations: {team: payments}\nversion: 1.0.0\n",
			opts: EditOptions{SetAnnotations: map[string]string{ImagesAnnotation: images}},
			expected: `name: mychart
annotations:
  team: payments
  helm.sh/images: |
    - name: backend
      image: registry/app:backend
version: 1.0.0
`,
		},
		{
			name:     "delete from empty annotations",
			data:     "name: mychart\nannotations: {}\n",
			opts:     EditOptions{DeleteAnnotations: []string{ImagesAnnotation}},
			expected: "name: mychart\nannotations: {}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Edit(context.Background(), []byte(tt.data), tt.opts)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

func TestUpdateImages(t *testing.T) {
	const annotation = "- name: author\n  image: nginx:1\n  whitelisted: true\n- name: backend\n  image: registry/app:old\n"

	tests := []struct {
		name        string
		value       string
		remove      []string
		images      []Image
		expected    string
		expectedErr string
	}{
		{
			name:     "replace owned and keep author entries",
			value:    annotation,
			remove:   []string{"backend"},
			images:   []Image{{Name: "frontend", Image: "registry/app:frontend"}},
			expected: "- name: author\n  image: nginx:1\n  whitelisted: true\n- name: frontend\n  image: registry/app:frontend\n",
		},
		{
			name:     "remove all owned",
			value:    "- name: backend\n  image: registry/app:old\n",
			remove:   []string{"backend"},
			expected: "",
		},
		{
			name:     "nothing to remove or add",
			value:    "- {name: author, image: 'nginx:1'}\n",
			remove:   []string{"backend"},
			expected: "- {name: author, image: 'nginx:1'}\n",
		},
		{
			name:        "collision with author entry",
			value:       annotation,
			remove:      []string{"backend"},
			images:      []Image{{Name: "author", Image: "registry/app:author"}},
			expectedErr: `helm.sh/images in Chart.yaml already has an entry "author"; remove it from Chart.yaml or rename the werf image`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := UpdateImages(context.Background(), tt.value, tt.remove, tt.images)
			if tt.expectedErr != "" {
				require.EqualError(t, err, tt.expectedErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
