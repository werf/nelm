package util

import (
	"context"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/werf/nelm/pkg/log"
)

func NewRestyClient(ctx context.Context) *resty.Client {
	client := resty.New().
		SetLogger(log.NewRestyLogger(ctx)).
		SetTimeout(30 * time.Second).
		SetRetryCount(2).
		SetRedirectPolicy(resty.FlexibleRedirectPolicy(20)).
		AddRetryCondition(
			func(r *resty.Response, err error) bool {
				return err != nil || r.StatusCode() >= 500
			},
		)

	switch log.Default.Level(ctx) {
	case log.TraceLevel, log.DebugLevel:
		client.SetDebug(true)
	default:
		client.SetDisableWarn(true)
	}

	return client
}
