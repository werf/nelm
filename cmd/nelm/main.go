package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/chanced/caps"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	helm_v3 "github.com/werf/3p-helm/cmd/helm"
	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/logboek"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/resrcchangcalc"
)

func main() {
	ctx := logboek.NewContext(context.Background(), logboek.DefaultLogger())

	cli.FlagEnvVarsPrefix = caps.ToScreamingSnake(common.Brand) + "_"
	afterAllCommandsBuiltFuncs := make(map[*cobra.Command]func(cmd *cobra.Command) error)

	// Needed for embedding original Helm 3 commands.
	var err error
	helmRootCmd, err = helm_v3.Init()
	if err != nil {
		abort(ctx, fmt.Errorf("init helm: %w", err), 1)
	}

	rootCmd := NewRootCommand(ctx, afterAllCommandsBuiltFuncs)

	for cmd, fn := range afterAllCommandsBuiltFuncs {
		if err := fn(cmd); err != nil {
			abort(ctx, err, 1)
		}
	}

	if unsupportedEnvVars := cli.FindUndefinedFlagEnvVarsInEnviron(); len(unsupportedEnvVars) > 0 {
		abort(ctx, fmt.Errorf("unsupported environment variable(s): %s", strings.Join(unsupportedEnvVars, ",")), 1)
	}

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		var exitCode int
		if errors.Is(err, resrcchangcalc.ErrChangesPlanned) {
			exitCode = 2
		} else {
			exitCode = 1
		}

		abort(ctx, err, exitCode)
	}
}

func abort(ctx context.Context, err error, exitCode int) {
	log.Default.WarnPop(ctx, "final")
	log.Default.Error(ctx, "Error: %s", err)
	os.Exit(exitCode)
}
