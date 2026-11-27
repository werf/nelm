package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	"github.com/werf/nelm/v2/pkg/log"
)

var helmRootCmd *cobra.Command

func allowedLogColorModesHelp() string {
	return "Allowed: " + strings.Join(log.LogColorModes, ", ")
}

func allowedLogLevelsHelp() string {
	return "Allowed: " + strings.Join(lo.Map(log.Levels, func(lvl log.Level, _ int) string {
		return string(lvl)
	}), ", ")
}

// The first SIGINT or SIGTERM cancels the returned context, which lets the action run its failure
// plan and auto-rollback. The second one exits the process right away.
func cancelOnFirstSignal(ctx context.Context) context.Context {
	ctx, ctxCancelFn := context.WithCancelCause(ctx)

	signalCh := make(chan os.Signal, 2)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-signalCh
		ctxCancelFn(fmt.Errorf("context canceled: received signal %q", sig.String()))

		go log.Default.Warn(ctx, "Received signal %q, canceling. Send it again to exit immediately", sig.String())

		sig = <-signalCh
		os.Exit(128 + int(sig.(syscall.Signal)))
	}()

	return ctx
}
