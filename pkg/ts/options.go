package ts

import (
	"context"

	"github.com/werf/nelm/pkg/common"
)

var tsOptionsKey chartTSOptionsKey

type chartTSOptionsKey struct{}

func GetTSOptionsFromContext(ctx context.Context) common.TypeScriptOptions {
	opts, ok := ctx.Value(tsOptionsKey).(common.TypeScriptOptions)
	if !ok {
		return common.TypeScriptOptions{}
	}

	return opts
}

func NewContextWithTSOptions(ctx context.Context, opts common.TypeScriptOptions) context.Context {
	return context.WithValue(ctx, tsOptionsKey, opts)
}
