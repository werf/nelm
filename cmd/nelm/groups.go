package main

import (
	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
)

var releaseCmdGroup = &cobra.Group{
	ID:    "release",
	Title: "Release commands:",
}

var chartCmdGroup = &cobra.Group{
	ID:    "chart",
	Title: "Chart commands:",
}

var repoCmdGroup = &cobra.Group{
	ID:    "repo",
	Title: "Repo commands:",
}

var (
	mainFlagGroup           = cli.NewFlagGroup("main", "Options:", 100)
	valuesFlagGroup         = cli.NewFlagGroup("values", "Values options:", 90)
	secretFlagGroup         = cli.NewFlagGroup("secret", "Secret options:", 80)
	patchFlagGroup          = cli.NewFlagGroup("patch", "Patch options:", 70)
	progressFlagGroup       = cli.NewFlagGroup("progress", "Progress options:", 65)
	chartRepoFlagGroup      = cli.NewFlagGroup("chart-repo", "Chart repository options:", 60)
	kubeConnectionFlagGroup = cli.NewFlagGroup("kube-connection", "Kubernetes connection options:", 50)
	performanceFlagGroup    = cli.NewFlagGroup("performance", "Performance options:", 40)
	miscFlagGroup           = cli.NewFlagGroup("misc", "Miscellaneous options:", 0)
)
