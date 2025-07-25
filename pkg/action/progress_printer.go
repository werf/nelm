package action

import (
	"context"
	"time"
)

type progressTablePrinter struct {
	ctx      context.Context
	cancel   context.CancelFunc
	interval time.Duration
	callback func()
	finished chan bool
}

func newProgressTablePrinter(ctx context.Context, interval time.Duration, callback func()) *progressTablePrinter {
	return &progressTablePrinter{
		ctx:      ctx,
		interval: interval,
		callback: callback,
		finished: make(chan bool),
	}
}

func (p *progressTablePrinter) Start() {
	p.ctx, p.cancel = context.WithCancel(p.ctx)
	// Cancel function is called inside the goroutine below.

	go func() {
		defer p.finish()
		defer p.cancel()

		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		for {
			select {
			case <-p.ctx.Done():
				p.callback()
				return
			case <-ticker.C:
				p.callback()
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
	if p.cancel != nil {
		<-p.finished // Wait for graceful stop
	}
}

func (p *progressTablePrinter) finish() {
	p.finished <- true // Used for graceful stop
}
