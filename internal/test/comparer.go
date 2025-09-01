package test

import (
	"regexp"

	"github.com/davecgh/go-spew/spew"
	"github.com/dominikbraun/graph"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/samber/lo"

	"github.com/werf/nelm/internal/resource"
)

func CompareResourceMetadataOption(releaseNamespace string) cmp.Option {
	return cmp.FilterPath(func(p cmp.Path) bool {
		if len(p) < 2 {
			return false
		}

		return p.Index(len(p)-2).String() == ".Object" && p.Index(len(p)-1).String() == `["metadata"]`
	}, cmp.Transformer("CleanMetadata", func(m interface{}) interface{} {
		metadata, ok := m.(map[string]interface{})
		if !ok {
			return m
		}

		cleanMetadata := lo.PickByKeys(metadata, []string{
			"name", "namespace", "labels", "annotations",
		})

		if ns, ok := cleanMetadata["namespace"]; ok {
			if ns == releaseNamespace || ns == "" {
				delete(cleanMetadata, "namespace")
			}
		}

		return cleanMetadata
	}))
}

func CompareInternalDependencyOption() cmp.Option {
	sp := &spew.ConfigState{
		Indent:                  " ",
		DisablePointerAddresses: true,
		DisableCapacities:       true,
		SortKeys:                true,
		SpewKeys:                true,
	}

	return cmpopts.SortSlices(func(a, b *resource.InternalDependency) bool {
		return sp.Sdump(a) < sp.Sdump(b)
	})
}

func CompareRegexpOption() cmp.Option {
	return cmp.Comparer(func(a, b *regexp.Regexp) bool {
		if a == nil || b == nil {
			return a == b
		}

		return a.String() == b.String()
	})
}

func IgnoreEdgeOption() cmp.Option {
	return cmp.FilterValues(func(x, y graph.Edge[string]) bool { return true }, cmp.Ignore())
}
