//nolint:gosec,testpackage // Test files use 0644 for test fixtures; white-box test needs access to internal functions
package tschart

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
		var tempDir string

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

			It("should return error for file path", func() {
				filePath := filepath.Join(tempDir, "chart.tgz")
				Expect(os.WriteFile(filePath, []byte("dummy"), 0o644)).To(Succeed())

				err := transformer.TransformChartDir(ctx, filePath)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("is not a directory"))
			})
		})

		Context("when chart has no ts/ directory", func() {
			It("should skip transformation silently", func() {
				chartPath := filepath.Join(tempDir, "my-chart")
				Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

				err := transformer.TransformChartDir(ctx, chartPath)
				Expect(err).NotTo(HaveOccurred())

				// No vendor bundle should be created
				vendorPath := filepath.Join(chartPath, VendorBundleFile)
				_, err = os.Stat(vendorPath)
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})

		Context("when chart has ts/ directory but no entrypoint", func() {
			It("should skip transformation silently", func() {
				chartPath := filepath.Join(tempDir, "my-chart")
				tsDir := filepath.Join(chartPath, "ts", "src")
				Expect(os.MkdirAll(tsDir, 0o755)).To(Succeed())

				Expect(os.WriteFile(
					filepath.Join(tsDir, "helpers.ts"),
					[]byte("export const foo = 'bar';"),
					0o644,
				)).To(Succeed())

				err := transformer.TransformChartDir(ctx, chartPath)
				Expect(err).NotTo(HaveOccurred())

				vendorPath := filepath.Join(chartPath, VendorBundleFile)
				_, err = os.Stat(vendorPath)
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})

		Context("when chart has TypeScript entrypoint but no node_modules", func() {
			It("should skip vendor bundle creation", func() {
				chartPath := filepath.Join(tempDir, "my-chart")
				tsDir := filepath.Join(chartPath, "ts", "src")
				Expect(os.MkdirAll(tsDir, 0o755)).To(Succeed())

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
					0o644,
				)).To(Succeed())

				err := transformer.TransformChartDir(ctx, chartPath)
				Expect(err).NotTo(HaveOccurred())

				// No vendor bundle should be created since there's no node_modules
				vendorPath := filepath.Join(chartPath, VendorBundleFile)
				_, err = os.Stat(vendorPath)
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})

		Context("when chart has TypeScript with fake node_modules", func() {
			It("should create vendor bundle with dependencies", func() {
				chartPath := filepath.Join(tempDir, "my-chart")
				tsDir := filepath.Join(chartPath, "ts", "src")
				Expect(os.MkdirAll(tsDir, 0o755)).To(Succeed())

				// Create source that imports a fake module
				Expect(os.WriteFile(
					filepath.Join(tsDir, "index.ts"),
					[]byte(`
						import { helper } from 'fake-lib';
						export function render(context: any) {
							return { manifests: [helper(context)] };
						}
					`),
					0o644,
				)).To(Succeed())

				// Create fake node_modules
				fakeLibDir := filepath.Join(chartPath, "ts", "node_modules", "fake-lib")
				Expect(os.MkdirAll(fakeLibDir, 0o755)).To(Succeed())

				Expect(os.WriteFile(
					filepath.Join(fakeLibDir, "package.json"),
					[]byte(`{"name": "fake-lib", "version": "1.0.0", "main": "index.js"}`),
					0o644,
				)).To(Succeed())

				Expect(os.WriteFile(
					filepath.Join(fakeLibDir, "index.js"),
					[]byte(`
						module.exports.helper = function(ctx) {
							return { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: ctx.Release.Name } };
						};
					`),
					0o644,
				)).To(Succeed())

				err := transformer.TransformChartDir(ctx, chartPath)
				Expect(err).NotTo(HaveOccurred())

				// Vendor bundle should be created
				vendorPath := filepath.Join(chartPath, VendorBundleFile)
				vendorContent, err := os.ReadFile(vendorPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(vendorContent)).To(ContainSubstring("__NELM_VENDOR__"))
				Expect(string(vendorContent)).To(ContainSubstring("fake-lib"))
			})
		})

		Context("when TypeScript has syntax errors", func() {
			It("should return formatted error", func() {
				chartPath := filepath.Join(tempDir, "my-chart")
				tsDir := filepath.Join(chartPath, "ts", "src")
				Expect(os.MkdirAll(tsDir, 0o755)).To(Succeed())

				// Create node_modules to trigger vendor bundle build
				nodeModulesDir := filepath.Join(chartPath, "ts", "node_modules", "some-lib")
				Expect(os.MkdirAll(nodeModulesDir, 0o755)).To(Succeed())
				Expect(os.WriteFile(
					filepath.Join(nodeModulesDir, "package.json"),
					[]byte(`{"name": "some-lib", "version": "1.0.0", "main": "index.js"}`),
					0o644,
				)).To(Succeed())
				Expect(os.WriteFile(
					filepath.Join(nodeModulesDir, "index.js"),
					[]byte(`module.exports = {};`),
					0o644,
				)).To(Succeed())

				Expect(os.WriteFile(
					filepath.Join(tsDir, "index.ts"),
					[]byte(`
						import 'some-lib';
						export function render(context: any) {
							return { manifests: [
						}
					`),
					0o644,
				)).To(Succeed())

				err := transformer.TransformChartDir(ctx, chartPath)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("TypeScript transpilation failed"))
			})
		})

		Context("when chart has multiple TypeScript files with imports", func() {
			It("should detect all dependencies", func() {
				chartPath := filepath.Join(tempDir, "my-chart")
				tsDir := filepath.Join(chartPath, "ts", "src")
				Expect(os.MkdirAll(tsDir, 0o755)).To(Succeed())

				// Create entrypoint
				Expect(os.WriteFile(
					filepath.Join(tsDir, "index.ts"),
					[]byte(`
						import { helper } from './helpers';
						import { util } from 'fake-util';
						export function render(context: any) {
							return { manifests: [helper(context, util)] };
						}
					`),
					0o644,
				)).To(Succeed())

				Expect(os.WriteFile(
					filepath.Join(tsDir, "helpers.ts"),
					[]byte(`
						export function helper(context: any, util: any) {
							return {
								apiVersion: 'v1',
								kind: 'ConfigMap',
								metadata: { name: context.Release.Name }
							};
						}
					`),
					0o644,
				)).To(Succeed())

				// Create fake node_modules
				fakeUtilDir := filepath.Join(chartPath, "ts", "node_modules", "fake-util")
				Expect(os.MkdirAll(fakeUtilDir, 0o755)).To(Succeed())

				Expect(os.WriteFile(
					filepath.Join(fakeUtilDir, "package.json"),
					[]byte(`{"name": "fake-util", "version": "1.0.0", "main": "index.js"}`),
					0o644,
				)).To(Succeed())

				Expect(os.WriteFile(
					filepath.Join(fakeUtilDir, "index.js"),
					[]byte(`module.exports.util = function() { return 'utility'; };`),
					0o644,
				)).To(Succeed())

				err := transformer.TransformChartDir(ctx, chartPath)
				Expect(err).NotTo(HaveOccurred())

				vendorPath := filepath.Join(chartPath, VendorBundleFile)
				vendorContent, err := os.ReadFile(vendorPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(vendorContent)).To(ContainSubstring("fake-util"))
			})
		})

		Context("when chart has JavaScript entrypoint (index.js)", func() {
			It("should work with JS entrypoint and node_modules", func() {
				chartPath := filepath.Join(tempDir, "my-chart")
				tsDir := filepath.Join(chartPath, "ts", "src")
				Expect(os.MkdirAll(tsDir, 0o755)).To(Succeed())

				Expect(os.WriteFile(
					filepath.Join(tsDir, "index.js"),
					[]byte(`
						const lib = require('js-lib');
						exports.render = function(context) {
							return { manifests: [{ apiVersion: 'v1', kind: 'Pod' }] };
						};
					`),
					0o644,
				)).To(Succeed())

				// Create fake node_modules
				jsLibDir := filepath.Join(chartPath, "ts", "node_modules", "js-lib")
				Expect(os.MkdirAll(jsLibDir, 0o755)).To(Succeed())

				Expect(os.WriteFile(
					filepath.Join(jsLibDir, "package.json"),
					[]byte(`{"name": "js-lib", "version": "1.0.0", "main": "index.js"}`),
					0o644,
				)).To(Succeed())

				Expect(os.WriteFile(
					filepath.Join(jsLibDir, "index.js"),
					[]byte(`module.exports = { hello: 'world' };`),
					0o644,
				)).To(Succeed())

				err := transformer.TransformChartDir(ctx, chartPath)
				Expect(err).NotTo(HaveOccurred())

				vendorPath := filepath.Join(chartPath, VendorBundleFile)
				vendorContent, err := os.ReadFile(vendorPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(vendorContent)).To(ContainSubstring("js-lib"))
			})
		})
	})

	Describe("extractPackageNames", func() {
		It("should extract regular packages from metafile", func() {
			metafile := `{
				"inputs": {
					"node_modules/lodash/index.js": {"bytes": 100},
					"node_modules/lodash/merge.js": {"bytes": 50},
					"node_modules/axios/lib/axios.js": {"bytes": 200},
					"src/index.ts": {"bytes": 500}
				}
			}`

			packages, err := extractPackageNames(metafile)
			Expect(err).NotTo(HaveOccurred())
			Expect(packages).To(ConsistOf("axios", "lodash"))
		})

		It("should extract scoped packages from metafile", func() {
			metafile := `{
				"inputs": {
					"node_modules/@types/node/index.d.ts": {"bytes": 100},
					"node_modules/@babel/core/lib/index.js": {"bytes": 200},
					"src/index.ts": {"bytes": 500}
				}
			}`

			packages, err := extractPackageNames(metafile)
			Expect(err).NotTo(HaveOccurred())
			Expect(packages).To(ConsistOf("@types/node", "@babel/core"))
		})

		It("should return empty list when no node_modules", func() {
			metafile := `{
				"inputs": {
					"src/index.ts": {"bytes": 500},
					"src/helpers.ts": {"bytes": 200}
				}
			}`

			packages, err := extractPackageNames(metafile)
			Expect(err).NotTo(HaveOccurred())
			Expect(packages).To(BeEmpty())
		})
	})

	Describe("extractPackagesFromVendorBundle", func() {
		It("should extract package names from vendor bundle", func() {
			bundle := `
				var __NELM_VENDOR__ = {};
				__NELM_VENDOR__['lodash'] = require('lodash');
				__NELM_VENDOR__['axios'] = require('axios');
				__NELM_VENDOR__['@types/node'] = require('@types/node');
			`

			packages := extractPackagesFromVendorBundle(bundle)
			Expect(packages).To(ConsistOf("lodash", "axios", "@types/node"))
		})

		It("should return empty list for bundle without packages", func() {
			bundle := `
				var __NELM_VENDOR__ = {};
				if (typeof global !== 'undefined') { global.__NELM_VENDOR__ = __NELM_VENDOR__; }
			`

			packages := extractPackagesFromVendorBundle(bundle)
			Expect(packages).To(BeEmpty())
		})
	})

	Describe("generateVendorEntrypoint", func() {
		It("should generate correct entrypoint", func() {
			packages := []string{"lodash", "axios"}
			entry := generateVendorEntrypoint(packages)

			Expect(entry).To(ContainSubstring("var __NELM_VENDOR__ = {};"))
			Expect(entry).To(ContainSubstring("__NELM_VENDOR__['lodash'] = require('lodash');"))
			Expect(entry).To(ContainSubstring("__NELM_VENDOR__['axios'] = require('axios');"))
			Expect(entry).To(ContainSubstring("global.__NELM_VENDOR__ = __NELM_VENDOR__"))
		})

		It("should handle empty package list", func() {
			packages := []string{}
			entry := generateVendorEntrypoint(packages)

			Expect(entry).To(ContainSubstring("var __NELM_VENDOR__ = {};"))
			Expect(entry).NotTo(ContainSubstring("require("))
		})
	})
})
