package action

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/gookit/color"
	"github.com/samber/lo"

	"github.com/werf/nelm/internal/track"
	"github.com/werf/nelm/pkg/log"
)

func newProgressPrinter() *progressPrinter {
	return &progressPrinter{}
}

type progressPrinter struct {
	ctxCancelFn context.CancelCauseFunc
	finishedCh  chan struct{}
}

func (p *progressPrinter) Start(ctx context.Context, interval time.Duration, tablesBuilder *track.TablesBuilder) {
	go func() {
		p.finishedCh = make(chan struct{})

		ctx, p.ctxCancelFn = context.WithCancelCause(ctx)
		defer func() {
			p.ctxCancelFn(fmt.Errorf("context canceled: table printer finished"))
			p.finishedCh <- struct{}{}
		}()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				printTables(ctx, tablesBuilder)
			case <-ctx.Done():
				printTables(ctx, tablesBuilder)
				return
			}
		}
	}()
}

func (p *progressPrinter) Stop() {
	p.ctxCancelFn(fmt.Errorf("context canceled: table printer stopped"))
}

func (p *progressPrinter) Wait() {
	<-p.finishedCh
}

func printTables(
	ctx context.Context,
	tablesBuilder *track.TablesBuilder,
) {
	maxTableWidth := log.Default.BlockContentWidth(ctx) - 2
	tablesBuilder.SetMaxTableWidth(maxTableWidth)

	if tables, nonEmpty := tablesBuilder.BuildEventTables(); nonEmpty {
		headers := lo.Keys(tables)
		sort.Strings(headers)

		for _, header := range headers {
			log.Default.InfoBlock(ctx, log.BlockOptions{
				BlockTitle: header,
			}, func() {
				log.Default.Info(ctx, tables[header].Render())
			})
		}
	}

	if tables, nonEmpty := tablesBuilder.BuildLogTables(); nonEmpty {
		headers := lo.Keys(tables)
		sort.Strings(headers)

		for _, header := range headers {
			log.Default.InfoBlock(ctx, log.BlockOptions{
				BlockTitle: header,
			}, func() {
				log.Default.Info(ctx, tables[header].Render())
			})
		}
	}

	if table, nonEmpty := tablesBuilder.BuildProgressTable(); nonEmpty {
		log.Default.InfoBlock(ctx, log.BlockOptions{
			BlockTitle: color.Style{color.Bold, color.Blue}.Render("Progress status"),
		}, func() {
			log.Default.Info(ctx, table.Render())
		})
	}
}
