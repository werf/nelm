package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadMergedKubernetesSchemas(t *testing.T) {
	const (
		commit = "0123456789abcdef0123456789abcdef01234567"
		repo   = "example/kubernetes-json-schema"
	)

	dirSHAs := map[string]string{
		"v1.37.1-standalone": "newest-tree",
		"v1.36.5-standalone": "older-tree",
	}

	newestPod := `{"properties":{"spec":{"properties":{"containers":{"type":"array"},"hostnameOverride":{"type":"string"}}}}}`
	olderPod := `{"properties":{"spec":{"properties":{"containers":{"type":"array"}}}}}`

	client := &http.Client{Transport: fakeUpstreamTransport{
		"https://api.github.com/repos/" + repo + "/git/trees/newest-tree": `{"tree":[
			{"path":"deployment-apps-v1.json","type":"blob"},
			{"path":"pod-v1.json","type":"blob"}
		]}`,
		"https://api.github.com/repos/" + repo + "/git/trees/older-tree": `{"tree":[
			{"path":"pod-v1.json","type":"blob"},
			{"path":"servicecidr-networking-v1beta1.json","type":"blob"}
		]}`,
		"https://raw.githubusercontent.com/" + repo + "/" + commit + "/v1.37.1-standalone/deployment-apps-v1.json":             `{"type":"object"}`,
		"https://raw.githubusercontent.com/" + repo + "/" + commit + "/v1.37.1-standalone/pod-v1.json":                         newestPod,
		"https://raw.githubusercontent.com/" + repo + "/" + commit + "/v1.36.5-standalone/pod-v1.json":                         olderPod,
		"https://raw.githubusercontent.com/" + repo + "/" + commit + "/v1.36.5-standalone/servicecidr-networking-v1beta1.json": `{"type":"object"}`,
	}}

	opts := options{kubernetesRepo: repo, parallelism: 2}

	files, err := downloadMergedKubernetesSchemas(context.Background(), client, opts, []string{"1.37.1", "1.36.5"}, dirSHAs, commit)
	require.NoError(t, err)

	contents := lo.SliceToMap(files, func(file schemaFile) (string, []byte) {
		return file.name, file.content
	})

	assert.ElementsMatch(t, []string{"deployment-apps-v1.json", "pod-v1.json", "servicecidr-networking-v1beta1.json"}, lo.Keys(contents),
		"the schemas only the older version has must be collected too")

	assert.JSONEq(t, newestPod, string(contents["pod-v1.json"]),
		"a schema present in several versions must be the one of the newest of them")
}
