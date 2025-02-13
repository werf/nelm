package main

import (
	"context"
	"os"

	"github.com/werf/logboek"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

func main() {
	ctx := logboek.NewContext(context.Background(), logboek.DefaultLogger())

	log.Default.WarnPush(ctx, "final", "Warning: %s CLI is in active development and is not recommended for general use. Command names, option names, option defaults are going to change, a lot.", common.Brand)

	rootCmd := BuildRootCommand(ctx)
	executeErr := rootCmd.ExecuteContext(ctx)

	log.Default.WarnPop(ctx, "final")

	if executeErr != nil {
		log.Default.Error(ctx, "Error: %s", executeErr)
		os.Exit(1)
	}
}
