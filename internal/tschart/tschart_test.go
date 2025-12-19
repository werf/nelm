package tschart

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	helmchart "github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chartutil"
)

func TestTSChart(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TSChart Integration Suite")
}

var _ = Describe("TSChart Integration Tests", func() {
	var (
		ctx     context.Context
		tempDir string
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "tschart-integration-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	Describe("Full Flow: TypeScript -> Render -> YAML", func() {
		It("should handle simple TypeScript with types", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			tsDir := filepath.Join(chartPath, "ts", "src")
			Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(
				filepath.Join(tsDir, "index.ts"),
				[]byte(`
export function render(context: any) {
	const releaseName: string = context.Release.Name;
	const replicas: number = context.Values.replicas || 1;

	return {
		manifests: [{
			apiVersion: 'v1',
			kind: 'ConfigMap',
			metadata: {
				name: releaseName + '-config',
				namespace: context.Release.Namespace
			},
			data: {
				replicas: String(replicas),
				message: 'Hello from TypeScript!'
			}
		}]
	};
}
				`),
				0644,
			)).To(Succeed())

			chart := &helmchart.Chart{
				Metadata: &helmchart.Metadata{
					Name:    "test-chart",
					Version: "1.0.0",
				},
				Files: []*helmchart.File{},
			}

			engine := NewEngine()
			values := chartutil.Values{
				"Values": map[string]interface{}{
					"replicas": 3,
				},
				"Release": map[string]interface{}{
					"Name":      "test-release",
					"Namespace": "default",
				},
			}

			renderedTemplates, err := engine.RenderFiles(ctx, chartPath, chart, values)
			Expect(err).NotTo(HaveOccurred())
			Expect(renderedTemplates).To(HaveKey(OutputFile))

			yaml := renderedTemplates[OutputFile]
			Expect(yaml).To(ContainSubstring("kind: ConfigMap"))
			Expect(yaml).To(ContainSubstring("name: test-release-config"))
			Expect(yaml).To(ContainSubstring("namespace: default"))
			Expect(yaml).To(ContainSubstring("replicas: \"3\""))
			Expect(yaml).To(ContainSubstring("message: Hello from TypeScript!"))
		})

		It("should handle module.exports.render pattern", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			tsDir := filepath.Join(chartPath, "ts", "src")
			Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(
				filepath.Join(tsDir, "index.ts"),
				[]byte(`
module.exports.render = function(context: any) {
	return {
		manifests: [{
			apiVersion: 'v1',
			kind: 'ConfigMap',
			metadata: { name: 'module-exports-test' }
		}]
	};
};
				`),
				0644,
			)).To(Succeed())

			chart := &helmchart.Chart{Files: []*helmchart.File{}}
			engine := NewEngine()
			values := chartutil.Values{}
			renderedTemplates, err := engine.RenderFiles(ctx, chartPath, chart, values)
			Expect(err).NotTo(HaveOccurred())

			yaml := renderedTemplates[OutputFile]
			Expect(yaml).To(ContainSubstring("kind: ConfigMap"))
			Expect(yaml).To(ContainSubstring("name: module-exports-test"))
		})

		It("should handle module.exports = { render } pattern", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			tsDir := filepath.Join(chartPath, "ts", "src")
			Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(
				filepath.Join(tsDir, "index.ts"),
				[]byte(`
module.exports = {
	render: function(context: any) {
		return {
			manifests: [{
				apiVersion: 'v1',
				kind: 'Secret',
				metadata: { name: 'object-pattern-test' }
			}]
		};
	}
};
				`),
				0644,
			)).To(Succeed())

			chart := &helmchart.Chart{Files: []*helmchart.File{}}
			engine := NewEngine()
			values := chartutil.Values{}
			renderedTemplates, err := engine.RenderFiles(ctx, chartPath, chart, values)
			Expect(err).NotTo(HaveOccurred())

			yaml := renderedTemplates[OutputFile]
			Expect(yaml).To(ContainSubstring("kind: Secret"))
			Expect(yaml).To(ContainSubstring("name: object-pattern-test"))
		})

		It("should handle TypeScript features (template literals, arrow functions)", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			tsDir := filepath.Join(chartPath, "ts", "src")
			Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(
				filepath.Join(tsDir, "index.ts"),
				[]byte(`
export const render = (context: any) => {
	const prefix = context.Release.Name;
	const resources = [1, 2, 3].map(i => ({
		apiVersion: 'v1',
		kind: 'ConfigMap',
		metadata: {
			name: prefix + '-config-' + i
		},
		data: {
			index: String(i)
		}
	}));

	return {
		manifests: resources
	};
};
				`),
				0644,
			)).To(Succeed())

			chart := &helmchart.Chart{Files: []*helmchart.File{}}
			engine := NewEngine()
			values := chartutil.Values{
				"Release": map[string]interface{}{
					"Name": "my-app",
				},
			}

			renderedTemplates, err := engine.RenderFiles(ctx, chartPath, chart, values)
			Expect(err).NotTo(HaveOccurred())

			yaml := renderedTemplates[OutputFile]
			Expect(yaml).To(ContainSubstring("name: my-app-config-1"))
			Expect(yaml).To(ContainSubstring("name: my-app-config-2"))
			Expect(yaml).To(ContainSubstring("name: my-app-config-3"))
			Expect(yaml).To(ContainSubstring("---"))
		})

		It("should handle TypeScript interfaces and types", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			tsDir := filepath.Join(chartPath, "ts", "src")
			Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(
				filepath.Join(tsDir, "index.ts"),
				[]byte(`
interface RenderContext {
	Release: {
		Name: string;
		Namespace: string;
	};
	Values: {
		replicas?: number;
	};
}

interface Manifest {
	apiVersion: string;
	kind: string;
	metadata: {
		name: string;
	};
	spec?: any;
}

export function render(context: RenderContext) {
	const manifest: Manifest = {
		apiVersion: 'apps/v1',
		kind: 'Deployment',
		metadata: {
			name: context.Release.Name
		},
		spec: {
			replicas: context.Values.replicas || 1
		}
	};

	return {
		manifests: [manifest]
	};
}
				`),
				0644,
			)).To(Succeed())

			chart := &helmchart.Chart{Files: []*helmchart.File{}}
			engine := NewEngine()
			values := chartutil.Values{
				"Release": map[string]interface{}{
					"Name":      "typed-app",
					"Namespace": "production",
				},
				"Values": map[string]interface{}{
					"replicas": 5,
				},
			}

			renderedTemplates, err := engine.RenderFiles(ctx, chartPath, chart, values)
			Expect(err).NotTo(HaveOccurred())

			yaml := renderedTemplates[OutputFile]
			Expect(yaml).To(ContainSubstring("kind: Deployment"))
			Expect(yaml).To(ContainSubstring("name: typed-app"))
			Expect(yaml).To(ContainSubstring("replicas: 5"))
		})
	})

	Describe("Error handling with sourcemaps", func() {
		It("should show TypeScript error with source location", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			tsDir := filepath.Join(chartPath, "ts", "src")
			Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(
				filepath.Join(tsDir, "index.ts"),
				[]byte(`
export function render(context: any) {
	// This will throw a runtime error
	const obj: any = null;
	obj.nonExistentProperty;

	return {
		manifests: []
	};
}
				`),
				0644,
			)).To(Succeed())

			chart := &helmchart.Chart{Files: []*helmchart.File{}}
			engine := NewEngine()
			values := chartutil.Values{}
			_, err := engine.RenderFiles(ctx, chartPath, chart, values)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("index.ts"))
			Expect(err.Error()).To(ContainSubstring("undefined"))
		})

		It("should show error when render function is missing", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			tsDir := filepath.Join(chartPath, "ts", "src")
			Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(
				filepath.Join(tsDir, "index.ts"),
				[]byte(`
export function notRender(context: any) {
	return { manifests: [] };
}
				`),
				0644,
			)).To(Succeed())

			chart := &helmchart.Chart{Files: []*helmchart.File{}}
			engine := NewEngine()
			values := chartutil.Values{}
			_, err := engine.RenderFiles(ctx, chartPath, chart, values)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not export 'render' function"))
		})

		// Note: esbuild doesn't perform type checking, only syntax/transpilation
		PIt("should show TypeScript type errors (skipped - esbuild doesn't type check)", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			tsDir := filepath.Join(chartPath, "ts", "src")
			Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(
				filepath.Join(tsDir, "index.ts"),
				[]byte(`
export function render(context: any) {
	const x: number = "not a number";
	return { manifests: [] };
}
				`),
				0644,
			)).To(Succeed())

			Expect(os.WriteFile(
				filepath.Join(chartPath, "ts", "tsconfig.json"),
				[]byte(`{"compilerOptions": {"strict": true}}`),
				0644,
			)).To(Succeed())

			chart := &helmchart.Chart{Files: []*helmchart.File{}}
			engine := NewEngine()
			values := chartutil.Values{}
			// esbuild doesn't check types, so this will succeed
			_, err := engine.RenderFiles(ctx, chartPath, chart, values)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Multiple files and imports", func() {
		It("should handle TypeScript with multiple files", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			tsDir := filepath.Join(chartPath, "ts", "src")
			Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(
				filepath.Join(tsDir, "index.ts"),
				[]byte(`
import { createConfigMap } from './helpers';

export function render(context: any) {
	return {
		manifests: [createConfigMap(context.Release.Name)]
	};
}
				`),
				0644,
			)).To(Succeed())

			Expect(os.WriteFile(
				filepath.Join(tsDir, "helpers.ts"),
				[]byte(`
export function createConfigMap(name: string) {
	return {
		apiVersion: 'v1',
		kind: 'ConfigMap',
		metadata: {
			name: name + '-config'
		},
		data: {
			source: 'helper-function'
		}
	};
}
				`),
				0644,
			)).To(Succeed())

			chart := &helmchart.Chart{Files: []*helmchart.File{}}
			engine := NewEngine()
			values := chartutil.Values{
				"Release": map[string]interface{}{
					"Name": "multi-file-app",
				},
			}

			renderedTemplates, err := engine.RenderFiles(ctx, chartPath, chart, values)
			Expect(err).NotTo(HaveOccurred())

			yaml := renderedTemplates[OutputFile]
			Expect(yaml).To(ContainSubstring("name: multi-file-app-config"))
			Expect(yaml).To(ContainSubstring("source: helper-function"))
		})
	})

	Describe("Inline sourcemaps", func() {
		It("should include inline sourcemaps for error reporting", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			tsDir := filepath.Join(chartPath, "ts", "src")
			Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

			// Create a file that will cause a runtime error
			Expect(os.WriteFile(
				filepath.Join(tsDir, "index.ts"),
				[]byte(`
export function render(context: any) {
	// Intentionally access undefined to trigger error with sourcemap
	const x: any = undefined;
	x.foo.bar; // This line should appear in error
	return { manifests: [] };
}
				`),
				0644,
			)).To(Succeed())

			chart := &helmchart.Chart{Files: []*helmchart.File{}}
			engine := NewEngine()
			values := chartutil.Values{}
			_, err := engine.RenderFiles(ctx, chartPath, chart, values)
			Expect(err).To(HaveOccurred())
			// The error should reference the original .ts file thanks to sourcemaps
			Expect(err.Error()).To(ContainSubstring("index.ts"))
		})
	})

	Describe("Packaged charts with source files", func() {
		It("should render from packaged chart source files", func() {
			// Create chart with source files (simulates packaged chart)
			sourceContent := `
export function render(context: any) {
	return {
		manifests: [{
			apiVersion: "v1",
			kind: "ConfigMap",
			metadata: { name: "packaged-source-test" }
		}]
	};
}
`
			chart := &helmchart.Chart{
				RuntimeFiles: []*helmchart.File{
					{Name: "ts/src/index.ts", Data: []byte(sourceContent)},
				},
			}

			engine := NewEngine()
			values := chartutil.Values{}
			// Use non-existent path to simulate packaged chart
			renderedTemplates, err := engine.RenderFiles(ctx, "./packaged-chart.tgz", chart, values)
			Expect(err).NotTo(HaveOccurred())

			yaml := renderedTemplates[OutputFile]
			Expect(yaml).To(ContainSubstring("name: packaged-source-test"))
		})

		It("should use vendor bundle from packaged chart for npm dependencies", func() {
			// Create a vendor bundle that provides a fake module
			vendorBundle := `
var __NELM_VENDOR_BUNDLE__ = (function() {
	var __NELM_VENDOR__ = {};
	__NELM_VENDOR__['fake-lib'] = {
		helper: function(name) {
			return { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: name + '-from-vendor' } };
		}
	};
	if (typeof global !== 'undefined') { global.__NELM_VENDOR__ = __NELM_VENDOR__; }
	return { __NELM_VENDOR__: __NELM_VENDOR__ };
})();
`
			sourceContent := `
const fakeLib = require('fake-lib');
export function render(context: any) {
	return {
		manifests: [fakeLib.helper(context.Release.Name)]
	};
}
`
			chart := &helmchart.Chart{
				RuntimeFiles: []*helmchart.File{
					{Name: VendorBundleFile, Data: []byte(vendorBundle)},
					{Name: "ts/src/index.ts", Data: []byte(sourceContent)},
				},
			}

			engine := NewEngine()
			values := chartutil.Values{
				"Release": map[string]interface{}{
					"Name": "vendor-test",
				},
			}
			renderedTemplates, err := engine.RenderFiles(ctx, "./packaged-chart.tgz", chart, values)
			Expect(err).NotTo(HaveOccurred())

			yaml := renderedTemplates[OutputFile]
			Expect(yaml).To(ContainSubstring("name: vendor-test-from-vendor"))
		})
	})

	Describe("npm dependencies with vendor bundle", func() {
		It("should render chart with npm dependencies from node_modules", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			tsDir := filepath.Join(chartPath, "ts", "src")
			Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

			// Create source that uses a fake npm module
			Expect(os.WriteFile(
				filepath.Join(tsDir, "index.ts"),
				[]byte(`
import { helper } from 'fake-lib';

export function render(context: any) {
	return {
		manifests: [helper(context.Release.Name)]
	};
}
				`),
				0644,
			)).To(Succeed())

			// Create fake node_modules
			fakeLibDir := filepath.Join(chartPath, "ts", "node_modules", "fake-lib")
			Expect(os.MkdirAll(fakeLibDir, 0755)).To(Succeed())

			Expect(os.WriteFile(
				filepath.Join(fakeLibDir, "package.json"),
				[]byte(`{"name": "fake-lib", "version": "1.0.0", "main": "index.js"}`),
				0644,
			)).To(Succeed())

			Expect(os.WriteFile(
				filepath.Join(fakeLibDir, "index.js"),
				[]byte(`
module.exports.helper = function(name) {
	return { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: name + '-from-npm' } };
};
				`),
				0644,
			)).To(Succeed())

			chart := &helmchart.Chart{Files: []*helmchart.File{}}
			engine := NewEngine()
			values := chartutil.Values{
				"Release": map[string]interface{}{
					"Name": "npm-test",
				},
			}

			renderedTemplates, err := engine.RenderFiles(ctx, chartPath, chart, values)
			Expect(err).NotTo(HaveOccurred())

			yaml := renderedTemplates[OutputFile]
			Expect(yaml).To(ContainSubstring("name: npm-test-from-npm"))
		})
	})
})
