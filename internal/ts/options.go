package ts

import "context"

var tsOptionsKey chartTSOptionsKey

type chartTSOptionsKey struct{}

type ChartTSOptions struct {
	DenoBinaryPath string
}

func GetTSOptionsFromContext(ctx context.Context) ChartTSOptions {
	opts, ok := ctx.Value(tsOptionsKey).(ChartTSOptions)
	if !ok {
		return ChartTSOptions{}
	}

	return opts
}

func NewContextWithTSOptions(ctx context.Context, opts ChartTSOptions) context.Context {
	return context.WithValue(ctx, tsOptionsKey, opts)
}
