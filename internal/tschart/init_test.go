//nolint:gosec,testpackage // Test files use 0644 for test fixtures; white-box test needs access to internal functions
package tschart

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Init", func() {
	var (
		ctx     context.Context
		tempDir string
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error

		tempDir, err = os.MkdirTemp("", "tschart-init-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	Describe("InitTSBoilerplate", func() {
		It("should create all expected files", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			err := InitTSBoilerplate(ctx, chartPath, "test-chart")
			Expect(err).NotTo(HaveOccurred())

			// Check ts/src/ files
			Expect(filepath.Join(chartPath, "ts", "src", "index.ts")).To(BeARegularFile())
			Expect(filepath.Join(chartPath, "ts", "src", "helpers.ts")).To(BeARegularFile())
			Expect(filepath.Join(chartPath, "ts", "src", "deployment.ts")).To(BeARegularFile())
			Expect(filepath.Join(chartPath, "ts", "src", "service.ts")).To(BeARegularFile())

			// Check ts/ root files
			Expect(filepath.Join(chartPath, "ts", "tsconfig.json")).To(BeARegularFile())
			Expect(filepath.Join(chartPath, "ts", "package.json")).To(BeARegularFile())
		})

		It("should create correct directory structure", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			err := InitTSBoilerplate(ctx, chartPath, "test-chart")
			Expect(err).NotTo(HaveOccurred())

			Expect(filepath.Join(chartPath, "ts")).To(BeADirectory())
			Expect(filepath.Join(chartPath, "ts", "src")).To(BeADirectory())
		})

		It("should substitute chart name in package.json", func() {
			chartPath := filepath.Join(tempDir, "my-custom-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			err := InitTSBoilerplate(ctx, chartPath, "my-custom-chart")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, "ts", "package.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring(`"name": "my-custom-chart"`))
			Expect(string(content)).To(ContainSubstring(`"description": "TypeScript chart for my-custom-chart"`))
		})

		It("should include render function in index.ts", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			err := InitTSBoilerplate(ctx, chartPath, "test-chart")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, "ts", "src", "index.ts"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("export function render"))
			Expect(string(content)).To(ContainSubstring("RenderContext"))
			Expect(string(content)).To(ContainSubstring("RenderResult"))
		})

		It("should include helper functions in helpers.ts", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			err := InitTSBoilerplate(ctx, chartPath, "test-chart")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, "ts", "src", "helpers.ts"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("export function getFullname"))
			Expect(string(content)).To(ContainSubstring("export function getLabels"))
			Expect(string(content)).To(ContainSubstring("export function getSelectorLabels"))
		})

		It("should include resource generators in separate files", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			err := InitTSBoilerplate(ctx, chartPath, "test-chart")
			Expect(err).NotTo(HaveOccurred())

			deploymentContent, err := os.ReadFile(filepath.Join(chartPath, "ts", "src", "deployment.ts"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(deploymentContent)).To(ContainSubstring("export function newDeployment"))

			serviceContent, err := os.ReadFile(filepath.Join(chartPath, "ts", "src", "service.ts"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(serviceContent)).To(ContainSubstring("export function newService"))
		})

		It("should include @nelm/types dependency in package.json", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			err := InitTSBoilerplate(ctx, chartPath, "test-chart")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, "ts", "package.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring(`"@nelm/types"`))
		})

		It("should include correct tsconfig.json options", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			err := InitTSBoilerplate(ctx, chartPath, "test-chart")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, "ts", "tsconfig.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring(`"target": "ES2015"`))
			Expect(string(content)).To(ContainSubstring(`"module": "CommonJS"`))
			Expect(string(content)).To(ContainSubstring(`"strict": true`))
			Expect(string(content)).To(ContainSubstring(`"declaration": true`))
		})

		It("should fail if ts/ directory already exists", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			tsDir := filepath.Join(chartPath, "ts")
			Expect(os.MkdirAll(tsDir, 0o755)).To(Succeed())

			err := InitTSBoilerplate(ctx, chartPath, "test-chart")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("TypeScript directory already exists"))
		})
	})

	Describe("InitChartStructure", func() {
		It("should create Chart.yaml, values.yaml, .helmignore", func() {
			chartPath := filepath.Join(tempDir, "ts-only-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			err := InitChartStructure(ctx, chartPath, "ts-only-chart")
			Expect(err).NotTo(HaveOccurred())

			Expect(filepath.Join(chartPath, "Chart.yaml")).To(BeARegularFile())
			Expect(filepath.Join(chartPath, "values.yaml")).To(BeARegularFile())
			Expect(filepath.Join(chartPath, ".helmignore")).To(BeARegularFile())
		})

		It("should NOT create charts/ directory", func() {
			chartPath := filepath.Join(tempDir, "ts-only-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			err := InitChartStructure(ctx, chartPath, "ts-only-chart")
			Expect(err).NotTo(HaveOccurred())

			Expect(filepath.Join(chartPath, "charts")).NotTo(BeADirectory())
		})

		It("should NOT create templates/ directory", func() {
			chartPath := filepath.Join(tempDir, "ts-only-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			err := InitChartStructure(ctx, chartPath, "ts-only-chart")
			Expect(err).NotTo(HaveOccurred())

			Expect(filepath.Join(chartPath, "templates")).NotTo(BeADirectory())
		})

		It("should substitute chart name in Chart.yaml", func() {
			chartPath := filepath.Join(tempDir, "my-ts-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			err := InitChartStructure(ctx, chartPath, "my-ts-chart")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, "Chart.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("name: my-ts-chart"))
		})

		It("should include TS entries in .helmignore", func() {
			chartPath := filepath.Join(tempDir, "ts-only-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			err := InitChartStructure(ctx, chartPath, "ts-only-chart")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, ".helmignore"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("ts/dist/"))
		})

		It("should skip existing Chart.yaml", func() {
			chartPath := filepath.Join(tempDir, "existing-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			existingContent := "apiVersion: v2\nname: existing-name\nversion: 1.0.0\n"
			Expect(os.WriteFile(filepath.Join(chartPath, "Chart.yaml"), []byte(existingContent), 0o644)).To(Succeed())

			err := InitChartStructure(ctx, chartPath, "new-name")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, "Chart.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(Equal(existingContent))
		})

		It("should skip existing values.yaml", func() {
			chartPath := filepath.Join(tempDir, "existing-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			existingContent := "myValue: 123\n"
			Expect(os.WriteFile(filepath.Join(chartPath, "values.yaml"), []byte(existingContent), 0o644)).To(Succeed())

			err := InitChartStructure(ctx, chartPath, "test-chart")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, "values.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(Equal(existingContent))
		})

		It("should enrich existing .helmignore with TS entries", func() {
			chartPath := filepath.Join(tempDir, "existing-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			existingContent := ".DS_Store\n.git/\n"
			Expect(os.WriteFile(filepath.Join(chartPath, ".helmignore"), []byte(existingContent), 0o644)).To(Succeed())

			err := InitChartStructure(ctx, chartPath, "test-chart")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, ".helmignore"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring(".DS_Store"))
			Expect(string(content)).To(ContainSubstring(".git/"))
			Expect(string(content)).To(ContainSubstring("ts/dist/"))
		})
	})

	Describe("EnsureGitignore", func() {
		It("should create minimal .gitignore if it does not exist", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

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
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			originalContent := "# My project\n*.log\n"
			Expect(os.WriteFile(filepath.Join(chartPath, ".gitignore"), []byte(originalContent), 0o644)).To(Succeed())

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
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			existingContent := "ts/node_modules/\nts/vendor/\nts/dist/\n"
			Expect(os.WriteFile(filepath.Join(chartPath, ".gitignore"), []byte(existingContent), 0o644)).To(Succeed())

			err := EnsureGitignore(chartPath)
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, ".gitignore"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(Equal(existingContent))
		})

		It("should add only missing entries", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			existingContent := "ts/node_modules/\n"
			Expect(os.WriteFile(filepath.Join(chartPath, ".gitignore"), []byte(existingContent), 0o644)).To(Succeed())

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
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			originalContent := "# Original content\n*.swp\n"
			Expect(os.WriteFile(filepath.Join(chartPath, ".helmignore"), []byte(originalContent), 0o644)).To(Succeed())

			err := AppendToHelmignore(chartPath)
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, ".helmignore"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(ContainSubstring("# Original content"))
			Expect(string(content)).To(ContainSubstring("*.swp"))
			Expect(string(content)).To(ContainSubstring("ts/dist/"))
		})

		It("should not duplicate entries if already present", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			existingContent := "# Existing\nts/dist/\n"
			Expect(os.WriteFile(filepath.Join(chartPath, ".helmignore"), []byte(existingContent), 0o644)).To(Succeed())

			err := AppendToHelmignore(chartPath)
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(chartPath, ".helmignore"))
			Expect(err).NotTo(HaveOccurred())
			// Should not have duplicates
			Expect(string(content)).To(Equal(existingContent))
		})

		It("should return error if .helmignore does not exist", func() {
			chartPath := filepath.Join(tempDir, "test-chart")
			Expect(os.MkdirAll(chartPath, 0o755)).To(Succeed())

			err := AppendToHelmignore(chartPath)
			Expect(err).To(HaveOccurred())
		})
	})
})
