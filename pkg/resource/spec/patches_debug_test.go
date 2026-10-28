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
		name := "diff"
		if render {
			name = "render"
		}

		t.Run(name, func(t *testing.T) {
			compile := func(patches []spec.Patch) ([]*spec.CompiledPatch, error) {
				if render {
					return spec.CompileRenderPatches(patches, varsRenderContext())
				}

				return spec.CompilePatches(patches)
			}
			apply := func(ctx context.Context, patches []*spec.CompiledPatch, res *spec.ResourceSpec) (*unstructured.Unstructured, error) {
				if !render {
					return spec.ApplyPatches(ctx, patches, res.ResourceMeta, "prod", res.Unstruct)
				}

				out, err := spec.BuildRenderPatchedResourceSpecs(ctx, "prod", []*spec.ResourceSpec{res}, patches)
				if err != nil {
					return nil, fmt.Errorf("apply render patches: %w", err)
				}

				return out[0].Unstruct, nil
			}

			for _, tt := range []struct {
				name    string
				program string
				wantLog string
			}{
				{
					name:    "debug passes through the whole resource",
					program: `debug`,
					wantLog: "[\"DEBUG:\",{\"apiVersion\":\"apps/v1\",\"kind\":\"Deployment\",\"metadata\":{\"name\":\"web\"},\"spec\":{\"replicas\":3}}]\n",
				},
				{
					name:    "debug filter emits messages without extra results",
					program: `debug("replicas=\(.spec.replicas) 100%", .spec, null, false, 9223372036854775808)`,
					wantLog: "[\"DEBUG:\",\"replicas=3 100%\"]\n[\"DEBUG:\",{\"replicas\":3}]\n[\"DEBUG:\",null]\n[\"DEBUG:\",false]\n[\"DEBUG:\",9223372036854775808]\n",
				},
				{
					name:    "debug preserves update paths",
					program: `.spec.replicas |= debug`,
					wantLog: "[\"DEBUG:\",3]\n",
				},
				{
					name:    "debug preserves path expressions",
					program: `debug(path(.spec.replicas | debug))`,
					wantLog: "[\"DEBUG:\",3]\n[\"DEBUG:\",[\"spec\",\"replicas\"]]\n",
				},
				{
					name:    "debug is not caught as an error",
					program: `try debug("safe") catch error("unexpected error")`,
					wantLog: "[\"DEBUG:\",\"safe\"]\n",
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
				require.Equal(t, "[\"DEBUG:\",\"replicas=4\"]\n[\"DEBUG:\",4]\n[\"DEBUG:\",6]\n", output.String())
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
							require.Contains(t, output.String(), `["DEBUG:",{"apiVersion":"apps/v1"`)
							require.Contains(t, output.String(), "[\"DEBUG:\",\"message\"]\n")
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
							require.Equal(t, fmt.Sprintf("[\"DEBUG:\",\"web-%d\"]\n[\"DEBUG:\",3]\n", i), output.String())
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
