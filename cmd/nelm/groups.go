package main

import (
	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/flag"
)

var releaseCmdGroup = &cobra.Group{
	ID:    "release",
	Title: "Release commands:",
}

var chartCmdGroup = &cobra.Group{
	ID:    "chart",
	Title: "Chart commands:",
}

var planCmdGroup = &cobra.Group{
	ID:    "plan",
	Title: "Plan commands:",
}

var repoCmdGroup = &cobra.Group{
	ID:    "repo",
	Title: "Repo commands:",
}

var (
	mainFlagGroup           = flag.NewGroup("main", "Options:", 100)
	valuesFlagGroup         = flag.NewGroup("values", "Values options:", 90)
	secretFlagGroup         = flag.NewGroup("secret", "Secret options:", 80)
	patchFlagGroup          = flag.NewGroup("patch", "Patch options:", 70)
	progressFlagGroup       = flag.NewGroup("progress", "Progress options:", 65)
	chartRepoFlagGroup      = flag.NewGroup("chart-repo", "Chart repository options:", 60)
	kubeConnectionFlagGroup = flag.NewGroup("kube-connection", "Kubernetes connection options:", 50)
	performanceFlagGroup    = flag.NewGroup("performance", "Performance options:", 40)
	miscFlagGroup           = flag.NewGroup("misc", "Miscellaneous options:", 0)
)
