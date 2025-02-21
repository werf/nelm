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

var mainFlagOptions = &flag.Group{
	ID:       "main",
	Title:    "Options:",
	Priority: 100,
}

var valuesFlagGroup = &flag.Group{
	ID:       "values",
	Title:    "Values options:",
	Priority: 90,
}

var secretFlagOptions = &flag.Group{
	ID:       "secret",
	Title:    "Secret options:",
	Priority: 80,
}

var patchFlagOptions = &flag.Group{
	ID:       "patch",
	Title:    "Patch options:",
	Priority: 70,
}

var progressFlagOptions = &flag.Group{
	ID:    "progress",
	Title: "Progress options:",
}

var chartRepoFlagGroup = &flag.Group{
	ID:       "chart-repo",
	Title:    "Chart repository options:",
	Priority: 60,
}

var kubeConnectionFlagOptions = &flag.Group{
	ID:       "kube-connection",
	Title:    "Kubernetes connection options:",
	Priority: 50,
}

var performanceFlagOptions = &flag.Group{
	ID:       "performance",
	Title:    "Performance options:",
	Priority: 40,
}

var miscFlagOptions = &flag.Group{
	ID:       "misc",
	Title:    "Miscellaneous options:",
	Priority: 0,
}
