package tschart

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHelpersModule_b64enc(t *testing.T) {
	manifestContent := `
const { b64enc } = require('nelm:helpers');

module.exports.render = function(context) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'Secret',
            data: {
                password: b64enc('mypassword')
            }
        }]
    };
};
`
	testChart := newTestChart(map[string]string{
		"ts/chart_render_main.js": manifestContent,
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	yaml := renderedTemplates["ts/chart_render_main.js"]
	assert.Contains(t, yaml, "password: bXlwYXNzd29yZA==")
}

func TestHelpersModule_b64dec(t *testing.T) {
	manifestContent := `
const { b64dec } = require('nelm:helpers');

module.exports.render = function(context) {
    const decoded = b64dec('aGVsbG8gd29ybGQ=');
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            data: {
                message: decoded
            }
        }]
    };
};
`
	testChart := newTestChart(map[string]string{
		"ts/chart_render_main.js": manifestContent,
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	yaml := renderedTemplates["ts/chart_render_main.js"]
	assert.Contains(t, yaml, "message: hello world")
}

func TestHelpersModule_sha256sum(t *testing.T) {
	manifestContent := `
const { sha256sum } = require('nelm:helpers');

module.exports.render = function(context) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            data: {
                hash: sha256sum('hello')
            }
        }]
    };
};
`
	testChart := newTestChart(map[string]string{
		"ts/chart_render_main.js": manifestContent,
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	yaml := renderedTemplates["ts/chart_render_main.js"]
	// SHA256 of "hello"
	assert.Contains(t, yaml, "hash: 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
}

func TestHelpersModule_bytesToString(t *testing.T) {
	manifestContent := `
const { bytesToString } = require('nelm:helpers');

module.exports.render = function(context) {
    // Simulate reading a file from context.Files
    const fileContent = context.Files['test.txt'];
    const textContent = bytesToString(fileContent);

    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            data: {
                content: textContent
            }
        }]
    };
};
`
	testChart := newTestChart(map[string]string{
		"ts/chart_render_main.js": manifestContent,
		"test.txt":     "Hello from file!",
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	yaml := renderedTemplates["ts/chart_render_main.js"]
	assert.Contains(t, yaml, "content: Hello from file!")
}

func TestHelpersModule_stringToBytes(t *testing.T) {
	manifestContent := `
const { stringToBytes, b64encBytes } = require('nelm:helpers');

module.exports.render = function(context) {
    const bytes = stringToBytes('hello');
    const encoded = b64encBytes(bytes);

    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'Secret',
            data: {
                value: encoded
            }
        }]
    };
};
`
	testChart := newTestChart(map[string]string{
		"ts/chart_render_main.js": manifestContent,
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	yaml := renderedTemplates["ts/chart_render_main.js"]
	assert.Contains(t, yaml, "value: aGVsbG8=")
}

func TestHelpersModule_b64encBytes(t *testing.T) {
	manifestContent := `
const { b64encBytes } = require('nelm:helpers');

module.exports.render = function(context) {
    // Simulate binary file content
    const fileContent = context.Files['binary.dat'];
    const encoded = b64encBytes(fileContent);

    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'Secret',
            data: {
                binary: encoded
            }
        }]
    };
};
`
	testChart := newTestChart(map[string]string{
		"ts/chart_render_main.js": manifestContent,
		"binary.dat":   "binary\x00data\x01here",
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	yaml := renderedTemplates["ts/chart_render_main.js"]
	assert.Contains(t, yaml, "binary:")
}

func TestHelpersModule_b64decBytes(t *testing.T) {
	manifestContent := `
const { b64decBytes, bytesToString } = require('nelm:helpers');

module.exports.render = function(context) {
    const bytes = b64decBytes('aGVsbG8gd29ybGQ=');
    const text = bytesToString(bytes);

    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            data: {
                message: text
            }
        }]
    };
};
`
	testChart := newTestChart(map[string]string{
		"ts/chart_render_main.js": manifestContent,
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	yaml := renderedTemplates["ts/chart_render_main.js"]
	assert.Contains(t, yaml, "message: hello world")
}

func TestHelpersModule_sha256sumBytes(t *testing.T) {
	manifestContent := `
const { sha256sumBytes } = require('nelm:helpers');

module.exports.render = function(context) {
    const fileContent = context.Files['data.txt'];
    const hash = sha256sumBytes(fileContent);

    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            data: {
                checksum: hash
            }
        }]
    };
};
`
	testChart := newTestChart(map[string]string{
		"ts/chart_render_main.js": manifestContent,
		"data.txt":     "hello",
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	yaml := renderedTemplates["ts/chart_render_main.js"]
	// SHA256 of "hello"
	assert.Contains(t, yaml, "checksum: 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
}
