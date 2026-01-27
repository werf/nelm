//nolint:testpackage // White-box test needs access to internal functions
package tschart

// DefaultOutputFile is the expected output path for TypeScript charts in tests.
// The actual output path in production is determined by the entrypoint found.
const DefaultOutputFile = "ts/src/index.ts"
