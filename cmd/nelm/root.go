package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/werf/nelm/pkg/common"
)

func BuildRootCommand(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:           strings.ToLower(common.Brand),
		Long:          fmt.Sprintf("%s is a Helm 3 replacement. %s manages and deploys Helm Charts to Kubernetes just like Helm, but provides a lot of features, improvements and bug fixes on top of what Helm 3 offers.", common.Brand, common.Brand),
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.SetUsageFunc(usageFunc)
	cmd.SetUsageTemplate(usageTemplate)

	cmd.AddGroup(
		ReleaseGroup,
		PlanGroup,
		ChartGroup,
	)

	cmd.AddCommand(BuildReleaseCommand(ctx))
	cmd.AddCommand(BuildPlanCommand(ctx))
	cmd.AddCommand(BuildChartCommand(ctx))

	return cmd
}
