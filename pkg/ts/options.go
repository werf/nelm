package ts

import (
	"context"

	"github.com/werf/nelm/pkg/helm/pkg/werf/helmopts"
)

var tsOptionsKey chartTSOptionsKey

type chartTSOptionsKey struct{}

func GetTSOptionsFromContext(ctx context.Context) helmopts.TypeScriptOptions {
	opts, ok := ctx.Value(tsOptionsKey).(helmopts.TypeScriptOptions)
	if !ok {
		return helmopts.TypeScriptOptions{}
	}

	return opts
}

func NewContextWithTSOptions(ctx context.Context, opts helmopts.TypeScriptOptions) context.Context {
	return context.WithValue(ctx, tsOptionsKey, opts)
}
