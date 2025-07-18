package action

import (
	"context"
	"time"
)

type progressTablePrinter struct {
	ctx      context.Context
	cancel   context.CancelFunc
	interval time.Duration
	timeout  time.Duration
	finished chan bool
}

func newProgressTablePrinter(ctx context.Context, interval, timeout time.Duration) *progressTablePrinter {
	return &progressTablePrinter{
		ctx:      ctx,
		interval: interval,
		timeout:  timeout,
		finished: make(chan bool),
	}
}

func (p *progressTablePrinter) Start(printFunc func()) {
	go func() {
		// Limit zero (infinity) timeout with 24 hours
		if p.timeout == 0 {
			p.timeout = time.Hour * 24
		}

		defer p.finish()

		p.ctx, p.cancel = context.WithTimeout(p.ctx, p.timeout)
		defer p.cancel()

		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		for {
			select {
			case <-p.ctx.Done():
				printFunc()
				return
			case <-ticker.C:
				printFunc()
			}
		}
	}()
}

func (p *progressTablePrinter) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
}

func (p *progressTablePrinter) Wait() {
	<-p.finished // Wait for graceful stop
}

func (p *progressTablePrinter) finish() {
	p.finished <- true // Used for graceful stop
}
