package spec_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/v2/pkg/log"
	"github.com/werf/nelm/v2/pkg/resource/spec"
)

func TestPatchesDebug(t *testing.T) {
	for _, render := range []bool{false, true} {
		patchKind := "diff"
		if render {
			patchKind = "render"
		}

		t.Run(patchKind, func(t *testing.T) {
			compile := func(patches []spec.Patch) ([]*spec.CompiledPatch, error) {
				if render {
					return spec.CompileRenderPatches(patches, varsRenderContext())
				}

				return spec.CompilePatches(patches)
			}
			apply := func(ctx context.Context, patches []*spec.CompiledPatch, res *spec.ResourceSpec) (*unstructured.Unstructured, error) {
				if !render {
					return spec.ApplyPatches(ctx, patches, res.ResourceMeta, "prod", res.Unstruct, spec.ApplyPatchesOptions{})
				}

				out, err := spec.BuildRenderPatchedResourceSpecs(ctx, "prod", []*spec.ResourceSpec{res}, patches)
				if err != nil {
					return nil, fmt.Errorf("apply render patches: %w", err)
				}

				return out[0].Unstruct, nil
			}

			header := fmt.Sprintf("Applying %s patch #1 to resource %q", patchKind, "Deployment/web")
			for _, tt := range []struct {
				name    string
				program string
				wantLog string
			}{
				{
					name:    "debug passes through the whole resource",
					program: `debug`,
					wantLog: header + ":\n" + `{
  "apiVersion": "apps/v1",
  "kind": "Deployment",
  "metadata": {
    "name": "web"
  },
  "spec": {
    "replicas": 3
  }
}
`,
				},
				{
					name:    "debug filter emits messages without extra results",
					program: `debug("replicas=\(.spec.replicas) 100%", .spec, null, false, 9223372036854775808)`,
					wantLog: header + ": replicas=3 100%\n" + header + ":\n{\n  \"replicas\": 3\n}\n" + header + ": null\n" + header + ": false\n" + header + ": 9223372036854775808\n",
				},
				{
					name:    "arrays and empty objects are indented",
					program: `debug([1, {nested: [true, null]}], {}, [])`,
					wantLog: header + ":\n[\n  1,\n  {\n    \"nested\": [\n      true,\n      null\n    ]\n  }\n]\n" + header + ":\n{}\n" + header + ":\n[]\n",
				},
				{
					name:    "strings are printed without JSON escaping",
					program: `debug("first\nsecond\t100%", "")`,
					wantLog: header + ": first\nsecond\t100%\n" + header + ": \n",
				},
				{
					name:    "numeric formatting follows gojq",
					program: `debug(nan, infinite, -infinite, 1.5)`,
					wantLog: header + ": null\n" + header + ": 1.7976931348623157e+308\n" + header + ": -1.7976931348623157e+308\n" + header + ": 1.5\n",
				},
				{
					name:    "debug preserves update paths",
					program: `.spec.replicas |= debug`,
					wantLog: header + ": 3\n",
				},
				{
					name:    "debug preserves path expressions",
					program: `debug(path(.spec.replicas | debug))`,
					wantLog: header + ": 3\n" + header + ":\n[\n  \"spec\",\n  \"replicas\"\n]\n",
				},
				{
					name:    "debug is not caught as an error",
					program: `try debug("safe") catch error("unexpected error")`,
					wantLog: header + ": safe\n",
				},
				{
					name:    "user definition overrides debug",
					program: `def debug: .; debug | debug("ignored")`,
				},
			} {
				t.Run(tt.name, func(t *testing.T) {
					ctx, output := patchDebugContext(t, log.DebugLevel)
					res := renderedSpec(t, "web", "", "myapp/templates/web.yaml", nil)
					original := res.Unstruct.DeepCopy()

					patches, err := compile([]spec.Patch{{Patch: tt.program}})
					require.NoError(t, err)
					require.Empty(t, output.String())

					out, err := apply(ctx, patches, res)
					require.NoError(t, err)
					require.Equal(t, original.Object, out.Object)
					require.Equal(t, original.Object, res.Unstruct.Object)
					require.Equal(t, tt.wantLog, output.String())
				})
			}

			t.Run("patches and filters chain in order", func(t *testing.T) {
				ctx, output := patchDebugContext(t, log.DebugLevel)
				res := renderedSpec(t, "web", "", "myapp/templates/web.yaml", nil)
				original := res.Unstruct.DeepCopy()

				second := `.spec.replicas += 2 | debug(.spec.replicas)`
				if render {
					second = `.spec.replicas += ($Values.replicaCount - 5) | debug(.spec.replicas)`
				}

				patches, err := compile([]spec.Patch{
					{Patch: `.spec.replicas += 1 | debug("replicas=\(.spec.replicas)") | .spec.replicas |= debug`},
					{Patch: second},
				})
				require.NoError(t, err)

				out, err := apply(ctx, patches, res)
				require.NoError(t, err)

				expected := original.DeepCopy()
				require.NoError(t, unstructured.SetNestedField(expected.Object, int64(6), "spec", "replicas"))
				require.Equal(t, expected.Object, out.Object)
				require.Equal(t, original.Object, res.Unstruct.Object)

				secondHeader := fmt.Sprintf("Applying %s patch #2 to resource %q", patchKind, "Deployment/web")
				require.Equal(t, header+": replicas=4\n"+header+": 4\n"+secondHeader+": 6\n", output.String())
			})

			t.Run("patch numbers include skipped rules", func(t *testing.T) {
				ctx, output := patchDebugContext(t, log.DebugLevel)
				res := renderedSpec(t, "web", "", "myapp/templates/web.yaml", nil)
				patches, err := compile([]spec.Patch{
					{Match: spec.ResourceMatcher{Names: []string{"other"}}, Patch: `debug("ignored")`},
					{Patch: `debug("matched")`},
				})
				require.NoError(t, err)

				out, err := apply(ctx, patches, res)
				require.NoError(t, err)
				require.Equal(t, res.Unstruct.Object, out.Object)
				require.Equal(t, fmt.Sprintf("Applying %s patch #2 to resource %q: matched\n", patchKind, "Deployment/web"), output.String())
			})

			t.Run("visibility follows the execution log level", func(t *testing.T) {
				patches, err := compile([]spec.Patch{{Patch: `debug | debug("message")`}})
				require.NoError(t, err)

				for _, lvl := range log.Levels {
					t.Run(string(lvl), func(t *testing.T) {
						ctx, output := patchDebugContext(t, lvl)
						res := renderedSpec(t, "web", "", "myapp/templates/web.yaml", nil)

						out, err := apply(ctx, patches, res)
						require.NoError(t, err)
						require.Equal(t, res.Unstruct.Object, out.Object)

						if lvl == log.DebugLevel || lvl == log.TraceLevel {
							require.Contains(t, output.String(), header+":\n{\n  \"apiVersion\": \"apps/v1\",")
							require.Contains(t, output.String(), header+": message\n")
						} else {
							require.Empty(t, output.String())
						}
					})
				}
			})

			t.Run("internal binding does not expose context values", func(t *testing.T) {
				type contextKey struct{}

				ctx, output := patchDebugContext(t, log.DebugLevel)
				ctx = context.WithValue(ctx, contextKey{}, "sensitive-context-value")
				res := renderedSpec(t, "web", "", "myapp/templates/web.yaml", nil)

				patches, err := compile([]spec.Patch{{Patch: `$__nelm_debug_context | tojson`}})
				require.NoError(t, err)

				_, err = apply(ctx, patches, res)
				require.Error(t, err)
				require.NotContains(t, err.Error(), "sensitive-context-value")
				require.Empty(t, output.String())
			})

			for _, parallel := range []bool{false, true} {
				name := "sequential reuse uses each execution context"
				if parallel {
					name = "parallel reuse uses each execution context"
				}

				t.Run(name, func(t *testing.T) {
					patches, err := compile([]spec.Patch{{Patch: `debug(.metadata.name) | .spec.replicas |= debug`}})
					require.NoError(t, err)

					var (
						group  errgroup.Group
						checks []func()
					)

					start := make(chan struct{})
					for i := range 24 {
						ctx, output := patchDebugContext(t, log.DebugLevel)
						res := renderedSpec(t, fmt.Sprintf("web-%d", i), "", "myapp/templates/web.yaml", nil)
						original := res.Unstruct.DeepCopy()

						var out *unstructured.Unstructured

						run := func() error {
							var err error

							out, err = apply(ctx, patches, res)

							return err
						}
						if parallel {
							group.Go(func() error {
								<-start

								return run()
							})
						} else {
							require.NoError(t, run())
						}

						checks = append(checks, func() {
							require.NotNil(t, out)
							require.Equal(t, original.Object, out.Object)
							require.Equal(t, original.Object, res.Unstruct.Object)

							header := fmt.Sprintf("Applying %s patch #1 to resource %q", patchKind, fmt.Sprintf("Deployment/web-%d", i))
							require.Equal(t, fmt.Sprintf("%s: web-%d\n%s: 3\n", header, i, header), output.String())
						})
					}

					close(start)
					require.NoError(t, group.Wait())

					for _, check := range checks {
						check()
					}
				})
			}
		})
	}
}
