package main

import (
	"github.com/spf13/cobra"

	"github.com/werf/nelm/pkg/flag"
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

var (
	mainFlagOptions           = flag.NewGroup("main", "Options:", 100)
	valuesFlagGroup           = flag.NewGroup("values", "Values options:", 90)
	secretFlagOptions         = flag.NewGroup("secret", "Secret options:", 80)
	patchFlagOptions          = flag.NewGroup("patch", "Patch options:", 70)
	progressFlagOptions       = flag.NewGroup("progress", "Progress options:", 65)
	chartRepoFlagGroup        = flag.NewGroup("chart-repo", "Chart repository options:", 60)
	kubeConnectionFlagOptions = flag.NewGroup("kube-connection", "Kubernetes connection options:", 50)
	performanceFlagOptions    = flag.NewGroup("performance", "Performance options:", 40)
	miscFlagOptions           = flag.NewGroup("misc", "Miscellaneous options:", 0)
)
