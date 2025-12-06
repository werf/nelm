package tschart

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	helmchart "github.com/werf/3p-helm/pkg/chart"
)

var _ = Describe("Transformer", func() {
	var (
		ctx         context.Context
		transformer *Transformer
	)

	BeforeEach(func() {
		ctx = context.Background()
		transformer = NewTransformer()
	})

	Describe("TransformChartDir", func() {
		var (
			tempDir string
		)

		BeforeEach(func() {
			var err error
			tempDir, err = os.MkdirTemp("", "tschart-test-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(tempDir)
		})

		Context("when chart path is not a directory", func() {
			It("should skip transformation for non-existent path", func() {
				err := transformer.TransformChartDir(ctx, "./non-existent-path")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should skip transformation for file path", func() {
				filePath := filepath.Join(tempDir, "chart.tgz")
				Expect(os.WriteFile(filePath, []byte("dummy"), 0644)).To(Succeed())

				err := transformer.TransformChartDir(ctx, filePath)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when chart has no ts/ directory", func() {
			It("should skip transformation silently", func() {
				chartPath := filepath.Join(tempDir, "my-chart")
				Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

				err := transformer.TransformChartDir(ctx, chartPath)
				Expect(err).NotTo(HaveOccurred())

				bundlePath := filepath.Join(chartPath, BundleFile)
				_, err = os.Stat(bundlePath)
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})

		Context("when chart has ts/ directory but no entrypoint", func() {
			It("should skip transformation silently", func() {
				chartPath := filepath.Join(tempDir, "my-chart")
				tsDir := filepath.Join(chartPath, "ts", "src")
				Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

				Expect(os.WriteFile(
					filepath.Join(tsDir, "helpers.ts"),
					[]byte("export const foo = 'bar';"),
					0644,
				)).To(Succeed())

				err := transformer.TransformChartDir(ctx, chartPath)
				Expect(err).NotTo(HaveOccurred())

				bundlePath := filepath.Join(chartPath, BundleFile)
				_, err = os.Stat(bundlePath)
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})

		Context("when chart has TypeScript entrypoint", func() {
			It("should transform and write bundle to disk", func() {
				chartPath := filepath.Join(tempDir, "my-chart")
				tsDir := filepath.Join(chartPath, "ts", "src")
				Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

				Expect(os.WriteFile(
					filepath.Join(tsDir, "index.ts"),
					[]byte(`
						export function render(context: any) {
							return {
								manifests: [{
									apiVersion: 'v1',
									kind: 'ConfigMap',
									metadata: { name: 'test' },
									data: { key: 'value' }
								}]
							};
						}
					`),
					0644,
				)).To(Succeed())

				err := transformer.TransformChartDir(ctx, chartPath)
				Expect(err).NotTo(HaveOccurred())

				bundlePath := filepath.Join(chartPath, BundleFile)
				bundleContent, err := os.ReadFile(bundlePath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(bundleContent)).To(ContainSubstring("render"))
				Expect(string(bundleContent)).To(ContainSubstring("ConfigMap"))
			})
		})

		Context("when bundle already exists", func() {
			It("should overwrite with fresh build", func() {
				chartPath := filepath.Join(tempDir, "my-chart")
				tsDir := filepath.Join(chartPath, "ts", "src")
				Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

				bundlePath := filepath.Join(chartPath, BundleFile)
				Expect(os.WriteFile(bundlePath, []byte("old bundle content"), 0644)).To(Succeed())

				Expect(os.WriteFile(
					filepath.Join(tsDir, "index.ts"),
					[]byte(`
						export function render(context: any) {
							return { manifests: [{ apiVersion: 'v1', kind: 'Secret' }] };
						}
					`),
					0644,
				)).To(Succeed())

				err := transformer.TransformChartDir(ctx, chartPath)
				Expect(err).NotTo(HaveOccurred())

				bundleContent, err := os.ReadFile(bundlePath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(bundleContent)).NotTo(Equal("old bundle content"))
				Expect(string(bundleContent)).To(ContainSubstring("Secret"))
			})
		})

		Context("when TypeScript has syntax errors", func() {
			It("should return formatted error", func() {
				chartPath := filepath.Join(tempDir, "my-chart")
				tsDir := filepath.Join(chartPath, "ts", "src")
				Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

				Expect(os.WriteFile(
					filepath.Join(tsDir, "index.ts"),
					[]byte(`
						export function render(context: any) {
							return { manifests: [
						}
					`),
					0644,
				)).To(Succeed())

				err := transformer.TransformChartDir(ctx, chartPath)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("TypeScript transpilation failed"))
			})
		})

		Context("when chart has multiple TypeScript files with imports", func() {
			It("should bundle all files together", func() {
				chartPath := filepath.Join(tempDir, "my-chart")
				tsDir := filepath.Join(chartPath, "ts", "src")
				Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

				// Create entrypoint
				Expect(os.WriteFile(
					filepath.Join(tsDir, "index.ts"),
					[]byte(`
						import { helper } from './helpers';
						export function render(context: any) {
							return { manifests: [helper(context)] };
						}
					`),
					0644,
				)).To(Succeed())

				Expect(os.WriteFile(
					filepath.Join(tsDir, "helpers.ts"),
					[]byte(`
						export function helper(context: any) {
							return {
								apiVersion: 'v1',
								kind: 'ConfigMap',
								metadata: { name: context.Release.Name }
							};
						}
					`),
					0644,
				)).To(Succeed())

				err := transformer.TransformChartDir(ctx, chartPath)
				Expect(err).NotTo(HaveOccurred())

				bundlePath := filepath.Join(chartPath, BundleFile)
				bundleContent, err := os.ReadFile(bundlePath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(bundleContent)).To(ContainSubstring("ConfigMap"))
				Expect(string(bundleContent)).To(ContainSubstring("sourceMappingURL"))
			})
		})

		Context("when chart has JavaScript entrypoint (index.js)", func() {
			It("should bundle successfully", func() {
				chartPath := filepath.Join(tempDir, "my-chart")
				tsDir := filepath.Join(chartPath, "ts", "src")
				Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

				Expect(os.WriteFile(
					filepath.Join(tsDir, "index.js"),
					[]byte(`
						exports.render = function(context) {
							return { manifests: [{ apiVersion: 'v1', kind: 'Pod' }] };
						};
					`),
					0644,
				)).To(Succeed())

				err := transformer.TransformChartDir(ctx, chartPath)
				Expect(err).NotTo(HaveOccurred())

				bundlePath := filepath.Join(chartPath, BundleFile)
				bundleContent, err := os.ReadFile(bundlePath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(bundleContent)).To(ContainSubstring("Pod"))
			})
		})
	})

	Describe("TransformChartForRender", func() {
		var (
			tempDir string
		)

		BeforeEach(func() {
			var err error
			tempDir, err = os.MkdirTemp("", "tschart-render-test-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(tempDir)
		})

		Context("when bundle already exists in chart.Files", func() {
			It("should skip transformation", func() {
				chart := &helmchart.Chart{
					Files: []*helmchart.File{
						{Name: BundleFile, Data: []byte("existing bundle")},
						{Name: "ts/src/index.ts", Data: []byte("export function render() {}")},
					},
				}

				err := transformer.TransformChartForRender(ctx, "./any-path", chart)
				Expect(err).NotTo(HaveOccurred())

				Expect(chart.Files).To(HaveLen(2))
			})
		})

		Context("when local directory has entrypoint but no bundle", func() {
			It("should build from directory and add to chart.Files", func() {
				chartPath := filepath.Join(tempDir, "my-chart")
				tsDir := filepath.Join(chartPath, "ts", "src")
				Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

				Expect(os.WriteFile(
					filepath.Join(tsDir, "index.ts"),
					[]byte(`
						export function render(context: any) {
							return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap' }] };
						}
					`),
					0644,
				)).To(Succeed())

				chart := &helmchart.Chart{
					Files: []*helmchart.File{},
				}

				err := transformer.TransformChartForRender(ctx, chartPath, chart)
				Expect(err).NotTo(HaveOccurred())

				Expect(chart.Files).To(HaveLen(1))
				Expect(chart.Files[0].Name).To(Equal(BundleFile))
				Expect(string(chart.Files[0].Data)).To(ContainSubstring("ConfigMap"))
			})
		})

		Context("when local directory has no TypeScript", func() {
			It("should skip transformation silently", func() {
				chartPath := filepath.Join(tempDir, "my-chart")
				Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

				chart := &helmchart.Chart{
					Files: []*helmchart.File{},
				}

				err := transformer.TransformChartForRender(ctx, chartPath, chart)
				Expect(err).NotTo(HaveOccurred())
				Expect(chart.Files).To(HaveLen(0))
			})
		})

		Context("when packaged chart has source but no bundle", func() {
			It("should return an error", func() {
				chart := &helmchart.Chart{
					Files: []*helmchart.File{
						{Name: "ts/src/index.ts", Data: []byte("export function render() {}")},
					},
				}

				err := transformer.TransformChartForRender(ctx, "./my-chart.tgz", chart)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("packaged chart has TypeScript source"))
				Expect(err.Error()).To(ContainSubstring("no pre-built bundle"))
			})
		})

		Context("when packaged chart has bundle", func() {
			It("should use existing bundle from chart.Files", func() {
				chart := &helmchart.Chart{
					Files: []*helmchart.File{
						{Name: BundleFile, Data: []byte("pre-built bundle")},
						{Name: "ts/src/index.ts", Data: []byte("export function render() {}")},
					},
				}

				err := transformer.TransformChartForRender(ctx, "./my-chart.tgz", chart)
				Expect(err).NotTo(HaveOccurred())
				Expect(chart.Files).To(HaveLen(2))
			})
		})

		Context("when packaged chart has no TypeScript", func() {
			It("should skip transformation silently", func() {
				chart := &helmchart.Chart{
					Files: []*helmchart.File{
						{Name: "README.md", Data: []byte("readme")},
					},
				}

				err := transformer.TransformChartForRender(ctx, "./my-chart.tgz", chart)
				Expect(err).NotTo(HaveOccurred())
				Expect(chart.Files).To(HaveLen(1))
			})
		})

		Context("when remote chart (oci://) has source but no bundle", func() {
			It("should return an error", func() {
				chart := &helmchart.Chart{
					Files: []*helmchart.File{
						{Name: "ts/src/index.ts", Data: []byte("export function render() {}")},
					},
				}

				err := transformer.TransformChartForRender(ctx, "oci://registry/my-chart", chart)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("packaged chart has TypeScript source"))
			})
		})

		Context("when TypeScript has syntax errors", func() {
			It("should return formatted error", func() {
				chartPath := filepath.Join(tempDir, "my-chart")
				tsDir := filepath.Join(chartPath, "ts", "src")
				Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())

				// Create entrypoint with syntax error
				Expect(os.WriteFile(
					filepath.Join(tsDir, "index.ts"),
					[]byte(`
						export function render(context: any) {
							return { manifests: [
						}
					`),
					0644,
				)).To(Succeed())

				chart := &helmchart.Chart{
					Files: []*helmchart.File{},
				}

				err := transformer.TransformChartForRender(ctx, chartPath, chart)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("TypeScript transpilation failed"))
			})
		})
	})
})
