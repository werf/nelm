package jsengine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHelpersModule_b64enc(t *testing.T) {
	manifestContent := `
const { b64enc } = require('helm:helpers');

exports.render = function(context) {
    return {
        apiVersion: 'v1',
        kind: 'Secret',
        data: {
            password: b64enc('mypassword')
        }
    };
};
`
	testChart := newTestChart(map[string]string{
		"js-templates/secret.manifest.js": manifestContent,
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := New()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	yaml := renderedTemplates["js-templates/secret.manifest.js"]
	assert.Contains(t, yaml, "password: bXlwYXNzd29yZA==")
}

func TestHelpersModule_b64dec(t *testing.T) {
	manifestContent := `
const { b64dec } = require('helm:helpers');

exports.render = function(context) {
    const decoded = b64dec('aGVsbG8gd29ybGQ=');
    return {
        apiVersion: 'v1',
        kind: 'ConfigMap',
        data: {
            message: decoded
        }
    };
};
`
	testChart := newTestChart(map[string]string{
		"js-templates/configmap.manifest.js": manifestContent,
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := New()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	yaml := renderedTemplates["js-templates/configmap.manifest.js"]
	assert.Contains(t, yaml, "message: hello world")
}

func TestHelpersModule_sha256sum(t *testing.T) {
	manifestContent := `
const { sha256sum } = require('helm:helpers');

exports.render = function(context) {
    return {
        apiVersion: 'v1',
        kind: 'ConfigMap',
        data: {
            hash: sha256sum('hello')
        }
    };
};
`
	testChart := newTestChart(map[string]string{
		"js-templates/configmap.manifest.js": manifestContent,
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := New()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	yaml := renderedTemplates["js-templates/configmap.manifest.js"]
	// SHA256 of "hello"
	assert.Contains(t, yaml, "hash: 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
}

func TestHelpersModule_quote(t *testing.T) {
	manifestContent := `
const { quote } = require('helm:helpers');

exports.render = function(context) {
    return {
        apiVersion: 'v1',
        kind: 'ConfigMap',
        data: {
            message: quote('hello world')
        }
    };
};
`
	testChart := newTestChart(map[string]string{
		"js-templates/configmap.manifest.js": manifestContent,
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := New()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	yaml := renderedTemplates["js-templates/configmap.manifest.js"]
	// The quote function should wrap in quotes, and YAML should preserve them
	assert.Contains(t, yaml, "message: '\"hello world\"'")
}

func TestHelpersModule_bytesToString(t *testing.T) {
	manifestContent := `
const { bytesToString } = require('helm:helpers');

exports.render = function(context) {
    // Simulate reading a file from context.Files
    const fileContent = context.Files['test.txt'];
    const textContent = bytesToString(fileContent);

    return {
        apiVersion: 'v1',
        kind: 'ConfigMap',
        data: {
            content: textContent
        }
    };
};
`
	testChart := newTestChart(map[string]string{
		"js-templates/configmap.manifest.js": manifestContent,
		"test.txt":                           "Hello from file!",
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := New()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	yaml := renderedTemplates["js-templates/configmap.manifest.js"]
	assert.Contains(t, yaml, "content: Hello from file!")
}

func TestHelpersModule_stringToBytes(t *testing.T) {
	manifestContent := `
const { stringToBytes, b64encBytes } = require('helm:helpers');

exports.render = function(context) {
    const bytes = stringToBytes('hello');
    const encoded = b64encBytes(bytes);

    return {
        apiVersion: 'v1',
        kind: 'Secret',
        data: {
            value: encoded
        }
    };
};
`
	testChart := newTestChart(map[string]string{
		"js-templates/secret.manifest.js": manifestContent,
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := New()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	yaml := renderedTemplates["js-templates/secret.manifest.js"]
	assert.Contains(t, yaml, "value: aGVsbG8=")
}

func TestHelpersModule_b64encBytes(t *testing.T) {
	manifestContent := `
const { b64encBytes } = require('helm:helpers');

exports.render = function(context) {
    // Simulate binary file content
    const fileContent = context.Files['binary.dat'];
    const encoded = b64encBytes(fileContent);

    return {
        apiVersion: 'v1',
        kind: 'Secret',
        data: {
            binary: encoded
        }
    };
};
`
	testChart := newTestChart(map[string]string{
		"js-templates/secret.manifest.js": manifestContent,
		"binary.dat":                      "binary\x00data\x01here",
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := New()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	yaml := renderedTemplates["js-templates/secret.manifest.js"]
	assert.Contains(t, yaml, "binary:")
}

func TestHelpersModule_b64decBytes(t *testing.T) {
	manifestContent := `
const { b64decBytes, bytesToString } = require('helm:helpers');

exports.render = function(context) {
    const bytes = b64decBytes('aGVsbG8gd29ybGQ=');
    const text = bytesToString(bytes);

    return {
        apiVersion: 'v1',
        kind: 'ConfigMap',
        data: {
            message: text
        }
    };
};
`
	testChart := newTestChart(map[string]string{
		"js-templates/configmap.manifest.js": manifestContent,
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := New()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	yaml := renderedTemplates["js-templates/configmap.manifest.js"]
	assert.Contains(t, yaml, "message: hello world")
}

func TestHelpersModule_sha256sumBytes(t *testing.T) {
	manifestContent := `
const { sha256sumBytes } = require('helm:helpers');

exports.render = function(context) {
    const fileContent = context.Files['data.txt'];
    const hash = sha256sumBytes(fileContent);

    return {
        apiVersion: 'v1',
        kind: 'ConfigMap',
        data: {
            checksum: hash
        }
    };
};
`
	testChart := newTestChart(map[string]string{
		"js-templates/configmap.manifest.js": manifestContent,
		"data.txt":                           "hello",
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := New()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	yaml := renderedTemplates["js-templates/configmap.manifest.js"]
	// SHA256 of "hello"
	assert.Contains(t, yaml, "checksum: 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
}
