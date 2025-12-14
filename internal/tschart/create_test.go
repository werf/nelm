package tschart

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Create", func() {
	var (
		ctx     context.Context
		tempDir string
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		tempDir, err = os.MkdirTemp("", "tschart-create-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	Describe("CreateTSBoilerplate", func() {
		It("should create all expected files", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			err := CreateTSBoilerplate(ctx, chartPath, "test-chart")
			Expect(err).NotTo(HaveOccurred())

			// Check ts/src/ files
			Expect(filepath.Join(chartPath, "ts", "src", "index.ts")).To(BeARegularFile())
			Expect(filepath.Join(chartPath, "ts", "src", "helpers.ts")).To(BeARegularFile())
			Expect(filepath.Join(chartPath, "ts", "src", "resources.ts")).To(BeARegularFile())

			// Check ts/types/ files
			Expect(filepath.Join(chartPath, "ts", "types", "nelm.d.ts")).To(BeARegularFile())

			// Check ts/ root files
			Expect(filepath.Join(chartPath, "ts", "tsconfig.json")).To(BeARegularFile())
			Expect(filepath.Join(chartPath, "ts", "package.json")).To(BeARegularFile())
			Expect(filepath.Join(chartPath, "ts", ".gitignore")).To(BeARegularFile())
		})

		It("should create correct directory structure", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			err := CreateTSBoilerplate(ctx, chartPath, "test-chart")
			Expect(err).NotTo(HaveOccurred())

			Expect(filepath.Join(chartPath, "ts")).To(BeADirectory())
			Expect(filepath.Join(chartPath, "ts", "src")).To(BeADirectory())
			Expect(filepath.Join(chartPath, "ts", "types")).To(BeADirectory())
		})

		It("should substitute chart name in package.json", func() {
			chartPath := filepath.Join(tempDir, "my-custom-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			err := CreateTSBoilerplate(ctx, chartPath, "my-custom-chart")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, "ts", "package.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring(`"name": "my-custom-chart"`))
			Expect(string(content)).To(ContainSubstring(`"description": "TypeScript chart for my-custom-chart"`))
		})

		It("should include render function in index.ts", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			err := CreateTSBoilerplate(ctx, chartPath, "test-chart")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, "ts", "src", "index.ts"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("export function render"))
			Expect(string(content)).To(ContainSubstring("RenderContext"))
			Expect(string(content)).To(ContainSubstring("RenderResult"))
		})

		It("should include helper functions in helpers.ts", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			err := CreateTSBoilerplate(ctx, chartPath, "test-chart")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, "ts", "src", "helpers.ts"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("export function fullname"))
			Expect(string(content)).To(ContainSubstring("export function labels"))
			Expect(string(content)).To(ContainSubstring("export function selectorLabels"))
		})

		It("should include resource generators in resources.ts", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			err := CreateTSBoilerplate(ctx, chartPath, "test-chart")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, "ts", "src", "resources.ts"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("export function newDeployment"))
			Expect(string(content)).To(ContainSubstring("export function newService"))
		})

		It("should include type definitions in nelm.d.ts", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			err := CreateTSBoilerplate(ctx, chartPath, "test-chart")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, "ts", "types", "nelm.d.ts"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("export interface RenderContext"))
			Expect(string(content)).To(ContainSubstring("export interface Release"))
			Expect(string(content)).To(ContainSubstring("export interface ChartMetadata"))
			Expect(string(content)).To(ContainSubstring("export interface RenderResult"))
		})

		It("should include correct tsconfig.json options", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			err := CreateTSBoilerplate(ctx, chartPath, "test-chart")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, "ts", "tsconfig.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring(`"target": "ES2015"`))
			Expect(string(content)).To(ContainSubstring(`"module": "CommonJS"`))
			Expect(string(content)).To(ContainSubstring(`"strict": true`))
			Expect(string(content)).To(ContainSubstring(`"declaration": true`))
		})

		It("should include correct .gitignore entries", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			err := CreateTSBoilerplate(ctx, chartPath, "test-chart")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, "ts", ".gitignore"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("vendor/"))
			Expect(string(content)).To(ContainSubstring("node_modules/"))
			Expect(string(content)).To(ContainSubstring("dist/"))
		})

		It("should overwrite existing ts/ directory", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			tsDir := filepath.Join(chartPath, "ts", "src")
			Expect(os.MkdirAll(tsDir, 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tsDir, "index.ts"), []byte("old content"), 0644)).To(Succeed())

			err := CreateTSBoilerplate(ctx, chartPath, "test-chart")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(tsDir, "index.ts"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).NotTo(Equal("old content"))
			Expect(string(content)).To(ContainSubstring("export function render"))
		})
	})

	Describe("CreateTSOnlyChartStructure", func() {
		It("should create Chart.yaml, values.yaml, .helmignore, and charts/", func() {
			chartPath := filepath.Join(tempDir, "ts-only-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			err := CreateTSOnlyChartStructure(ctx, chartPath, "ts-only-chart")
			Expect(err).NotTo(HaveOccurred())

			Expect(filepath.Join(chartPath, "Chart.yaml")).To(BeARegularFile())
			Expect(filepath.Join(chartPath, "values.yaml")).To(BeARegularFile())
			Expect(filepath.Join(chartPath, ".helmignore")).To(BeARegularFile())
			Expect(filepath.Join(chartPath, "charts")).To(BeADirectory())
		})

		It("should NOT create templates/ directory", func() {
			chartPath := filepath.Join(tempDir, "ts-only-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			err := CreateTSOnlyChartStructure(ctx, chartPath, "ts-only-chart")
			Expect(err).NotTo(HaveOccurred())

			Expect(filepath.Join(chartPath, "templates")).NotTo(BeADirectory())
		})

		It("should substitute chart name in Chart.yaml", func() {
			chartPath := filepath.Join(tempDir, "my-ts-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			err := CreateTSOnlyChartStructure(ctx, chartPath, "my-ts-chart")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, "Chart.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("name: my-ts-chart"))
		})

		It("should include TS entries in .helmignore", func() {
			chartPath := filepath.Join(tempDir, "ts-only-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			err := CreateTSOnlyChartStructure(ctx, chartPath, "ts-only-chart")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, ".helmignore"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("ts/node_modules/"))
			Expect(string(content)).To(ContainSubstring("ts/dist/"))
		})
	})

	Describe("EnsureGitignore", func() {
		It("should create minimal .gitignore if it does not exist", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			err := EnsureGitignore(chartPath)
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, ".gitignore"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("ts/node_modules/"))
			Expect(string(content)).To(ContainSubstring("ts/vendor/"))
			Expect(string(content)).To(ContainSubstring("ts/dist/"))
		})

		It("should append missing entries to existing .gitignore", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			originalContent := "# My project\n*.log\n"
			Expect(os.WriteFile(filepath.Join(chartPath, ".gitignore"), []byte(originalContent), 0644)).To(Succeed())

			err := EnsureGitignore(chartPath)
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, ".gitignore"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("# My project"))
			Expect(string(content)).To(ContainSubstring("*.log"))
			Expect(string(content)).To(ContainSubstring("ts/node_modules/"))
			Expect(string(content)).To(ContainSubstring("ts/vendor/"))
			Expect(string(content)).To(ContainSubstring("ts/dist/"))
		})

		It("should not duplicate entries if already present", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			existingContent := "ts/node_modules/\nts/vendor/\nts/dist/\n"
			Expect(os.WriteFile(filepath.Join(chartPath, ".gitignore"), []byte(existingContent), 0644)).To(Succeed())

			err := EnsureGitignore(chartPath)
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, ".gitignore"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(Equal(existingContent))
		})

		It("should add only missing entries", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			existingContent := "ts/node_modules/\n"
			Expect(os.WriteFile(filepath.Join(chartPath, ".gitignore"), []byte(existingContent), 0644)).To(Succeed())

			err := EnsureGitignore(chartPath)
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, ".gitignore"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("ts/node_modules/"))
			Expect(string(content)).To(ContainSubstring("ts/vendor/"))
			Expect(string(content)).To(ContainSubstring("ts/dist/"))
		})
	})

	Describe("AppendToHelmignore", func() {
		It("should append TS entries to existing .helmignore", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			originalContent := "# Original content\n*.swp\n"
			Expect(os.WriteFile(filepath.Join(chartPath, ".helmignore"), []byte(originalContent), 0644)).To(Succeed())

			err := AppendToHelmignore(chartPath)
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, ".helmignore"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("# Original content"))
			Expect(string(content)).To(ContainSubstring("*.swp"))
			Expect(string(content)).To(ContainSubstring("ts/node_modules/"))
			Expect(string(content)).To(ContainSubstring("ts/dist/"))
		})

		It("should not duplicate entries if already present", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			existingContent := "# Existing\nts/node_modules/\nts/dist/\n"
			Expect(os.WriteFile(filepath.Join(chartPath, ".helmignore"), []byte(existingContent), 0644)).To(Succeed())

			err := AppendToHelmignore(chartPath)
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, ".helmignore"))
			Expect(err).NotTo(HaveOccurred())
			// Should not have duplicates
			Expect(string(content)).To(Equal(existingContent))
		})

		It("should return error if .helmignore does not exist", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0755)).To(Succeed())

			err := AppendToHelmignore(chartPath)
			Expect(err).To(HaveOccurred())
		})
	})
})
