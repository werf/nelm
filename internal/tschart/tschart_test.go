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

	Describe("Full Flow: TypeScript -> Transform -> Render -> YAML", func() {
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

			transformer := NewTransformer()
			err := transformer.TransformChartForRender(ctx, chartPath, chart)
			Expect(err).NotTo(HaveOccurred())

			var bundleFile *helmchart.File
			for _, f := range chart.Files {
				if f.Name == BundleFile {
					bundleFile = f
					break
				}
			}
			Expect(bundleFile).NotTo(BeNil())
			Expect(bundleFile.Data).NotTo(BeEmpty())

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

			renderedTemplates, err := engine.RenderFiles(ctx, chart, values)
			Expect(err).NotTo(HaveOccurred())
			Expect(renderedTemplates).To(HaveKey(BundleFile))

			yaml := renderedTemplates[BundleFile]
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
			transformer := NewTransformer()
			err := transformer.TransformChartForRender(ctx, chartPath, chart)
			Expect(err).NotTo(HaveOccurred())

			engine := NewEngine()
			values := chartutil.Values{}
			renderedTemplates, err := engine.RenderFiles(ctx, chart, values)
			Expect(err).NotTo(HaveOccurred())

			yaml := renderedTemplates[BundleFile]
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
			transformer := NewTransformer()
			err := transformer.TransformChartForRender(ctx, chartPath, chart)
			Expect(err).NotTo(HaveOccurred())

			engine := NewEngine()
			values := chartutil.Values{}
			renderedTemplates, err := engine.RenderFiles(ctx, chart, values)
			Expect(err).NotTo(HaveOccurred())

			yaml := renderedTemplates[BundleFile]
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
			transformer := NewTransformer()
			err := transformer.TransformChartForRender(ctx, chartPath, chart)
			Expect(err).NotTo(HaveOccurred())

			engine := NewEngine()
			values := chartutil.Values{
				"Release": map[string]interface{}{
					"Name": "my-app",
				},
			}

			renderedTemplates, err := engine.RenderFiles(ctx, chart, values)
			Expect(err).NotTo(HaveOccurred())

			yaml := renderedTemplates[BundleFile]
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
			transformer := NewTransformer()
			err := transformer.TransformChartForRender(ctx, chartPath, chart)
			Expect(err).NotTo(HaveOccurred())

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

			renderedTemplates, err := engine.RenderFiles(ctx, chart, values)
			Expect(err).NotTo(HaveOccurred())

			yaml := renderedTemplates[BundleFile]
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
			transformer := NewTransformer()
			err := transformer.TransformChartForRender(ctx, chartPath, chart)
			Expect(err).NotTo(HaveOccurred())

			engine := NewEngine()
			values := chartutil.Values{}
			_, err = engine.RenderFiles(ctx, chart, values)
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
			transformer := NewTransformer()
			err := transformer.TransformChartForRender(ctx, chartPath, chart)
			Expect(err).NotTo(HaveOccurred())

			engine := NewEngine()
			values := chartutil.Values{}
			_, err = engine.RenderFiles(ctx, chart, values)
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
			transformer := NewTransformer()
			err := transformer.TransformChartForRender(ctx, chartPath, chart)
			// esbuild doesn't check types, so this will succeed
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
			transformer := NewTransformer()
			err := transformer.TransformChartForRender(ctx, chartPath, chart)
			Expect(err).NotTo(HaveOccurred())

			engine := NewEngine()
			values := chartutil.Values{
				"Release": map[string]interface{}{
					"Name": "multi-file-app",
				},
			}

			renderedTemplates, err := engine.RenderFiles(ctx, chart, values)
			Expect(err).NotTo(HaveOccurred())

			yaml := renderedTemplates[BundleFile]
			Expect(yaml).To(ContainSubstring("name: multi-file-app-config"))
			Expect(yaml).To(ContainSubstring("source: helper-function"))
		})
	})

	Describe("Inline sourcemaps", func() {
		It("should include inline sourcemaps in bundle", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			tsDir := filepath.Join(chartPath, "ts", "src")
			Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(
				filepath.Join(tsDir, "index.ts"),
				[]byte(`
export function render(context: any) {
	return { manifests: [] };
}
				`),
				0644,
			)).To(Succeed())

			chart := &helmchart.Chart{Files: []*helmchart.File{}}
			transformer := NewTransformer()
			err := transformer.TransformChartForRender(ctx, chartPath, chart)
			Expect(err).NotTo(HaveOccurred())

			var bundleFile *helmchart.File
			for _, f := range chart.Files {
				if f.Name == BundleFile {
					bundleFile = f
					break
				}
			}
			Expect(bundleFile).NotTo(BeNil())

			bundleContent := string(bundleFile.Data)
			Expect(bundleContent).To(ContainSubstring("//# sourceMappingURL=data:application/json;base64"))
		})
	})

	Describe("Pre-built bundle in chart.Files", func() {
		It("should use existing bundle without rebuilding", func() {
			// Create chart with pre-built bundle (simulates packaged chart)
			preBuildBundle := []byte(`
// Pre-built bundle
var __defProp = Object.defineProperty;
var __export = (target, all) => {
  for (var name in all)
    __defProp(target, name, { get: all[name], enumerable: true });
};
var src_exports = {};
__export(src_exports, {
  render: () => render
});
module.exports = src_exports;
function render(context) {
  return {
    manifests: [{
      apiVersion: "v1",
      kind: "ConfigMap",
      metadata: { name: "pre-built-bundle-test" }
    }]
  };
}
			`)

			chart := &helmchart.Chart{
				Files: []*helmchart.File{
					{Name: BundleFile, Data: preBuildBundle},
				},
			}

			transformer := NewTransformer()
			err := transformer.TransformChartForRender(ctx, "./any-path.tgz", chart)
			Expect(err).NotTo(HaveOccurred())

			Expect(chart.Files).To(HaveLen(1))

			engine := NewEngine()
			values := chartutil.Values{}
			renderedTemplates, err := engine.RenderFiles(ctx, chart, values)
			Expect(err).NotTo(HaveOccurred())

			yaml := renderedTemplates[BundleFile]
			Expect(yaml).To(ContainSubstring("name: pre-built-bundle-test"))
		})
	})
})
