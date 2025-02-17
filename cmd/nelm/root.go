package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/werf/nelm/pkg/common"
)

func NewRootCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:           strings.ToLower(common.Brand),
		Long:          fmt.Sprintf("%s is a Helm 3 replacement. %s manages and deploys Helm Charts to Kubernetes just like Helm, but provides a lot of features, improvements and bug fixes on top of what Helm 3 offers.", common.Brand, common.Brand),
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.SetUsageFunc(usageFunc)
	cmd.SetUsageTemplate(usageTemplate)

	cmd.AddGroup(
		releaseGroup,
		planGroup,
		chartGroup,
	)

	cmd.AddCommand(newReleaseCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newPlanCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
