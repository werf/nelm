package ts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/internal/ts"
)

func TestExtractPackageNames(t *testing.T) {
	t.Run("extracts regular packages", func(t *testing.T) {
		metafile := `{
			"inputs": {
				"node_modules/lodash/index.js": {"bytes": 100},
				"node_modules/lodash/merge.js": {"bytes": 50},
				"node_modules/axios/lib/axios.js": {"bytes": 200},
				"src/index.ts": {"bytes": 500}
			}
		}`

		packages, err := ts.ExtractPackageNames(metafile)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"axios", "lodash"}, packages)
	})

	t.Run("extracts scoped packages", func(t *testing.T) {
		metafile := `{
			"inputs": {
				"node_modules/@types/node/index.d.ts": {"bytes": 100},
				"node_modules/@babel/core/lib/index.js": {"bytes": 200},
				"src/index.ts": {"bytes": 500}
			}
		}`

		packages, err := ts.ExtractPackageNames(metafile)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"@types/node", "@babel/core"}, packages)
	})

	t.Run("returns empty for no node_modules", func(t *testing.T) {
		metafile := `{
			"inputs": {
				"src/index.ts": {"bytes": 500},
				"src/helpers.ts": {"bytes": 200}
			}
		}`

		packages, err := ts.ExtractPackageNames(metafile)
		require.NoError(t, err)
		assert.Empty(t, packages)
	})

	t.Run("handles nested node_modules paths (pnpm style)", func(t *testing.T) {
		metafile := `{
			"inputs": {
				"virtual:node_modules/.pnpm/lodash@4.17.21/node_modules/lodash/index.js": {"bytes": 100}
			}
		}`

		packages, err := ts.ExtractPackageNames(metafile)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"lodash"}, packages)
	})
}

func TestExtractPackagesFromVendorBundle(t *testing.T) {
	t.Run("extracts package names", func(t *testing.T) {
		bundle := `
			var __NELM_VENDOR__ = {};
			__NELM_VENDOR__['lodash'] = require('lodash');
			__NELM_VENDOR__['axios'] = require('axios');
			__NELM_VENDOR__['@types/node'] = require('@types/node');
		`

		packages := ts.ExtractPackagesFromVendorBundle(bundle)
		assert.ElementsMatch(t, []string{"lodash", "axios", "@types/node"}, packages)
	})

	t.Run("returns empty for no packages", func(t *testing.T) {
		bundle := `
			var __NELM_VENDOR__ = {};
			if (typeof global !== 'undefined') { global.__NELM_VENDOR__ = __NELM_VENDOR__; }
		`

		packages := ts.ExtractPackagesFromVendorBundle(bundle)
		assert.Empty(t, packages)
	})
}

func TestGenerateVendorEntrypoint(t *testing.T) {
	t.Run("generates correct entrypoint", func(t *testing.T) {
		packages := []string{"lodash", "axios"}
		entry := ts.GenerateVendorEntrypoint(packages)

		assert.Contains(t, entry, "var __NELM_VENDOR__ = {};")
		assert.Contains(t, entry, "__NELM_VENDOR__['lodash'] = require('lodash');")
		assert.Contains(t, entry, "__NELM_VENDOR__['axios'] = require('axios');")
		assert.Contains(t, entry, "global.__NELM_VENDOR__ = __NELM_VENDOR__")
	})

	t.Run("handles empty package list", func(t *testing.T) {
		entry := ts.GenerateVendorEntrypoint([]string{})

		assert.Contains(t, entry, "var __NELM_VENDOR__ = {};")
		assert.NotContains(t, entry, "require(")
	})
}
