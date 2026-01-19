package main

import (
	"github.com/werf/common-go/pkg/cli"
)

var (
	releaseCmdGroup    = cli.NewCommandGroup("release", "Release commands:", 100)
	chartCmdGroup      = cli.NewCommandGroup("chart", "Chart commands:", 90)
	secretCmdGroup     = cli.NewCommandGroup("secret", "Secret commands:", 80)
	dependencyCmdGroup = cli.NewCommandGroup("dependency", "Dependency commands:", 70)
	repoCmdGroup       = cli.NewCommandGroup("repo", "Repo commands:", 60)
	miscCmdGroup       = cli.NewCommandGroup("misc", "Other commands:", 0)

	mainFlagGroup           = cli.NewFlagGroup("main", "Options:", 100)
	valuesFlagGroup         = cli.NewFlagGroup("values", "Values options:", 90)
	secretFlagGroup         = cli.NewFlagGroup("secret", "Secret options:", 80)
	patchFlagGroup          = cli.NewFlagGroup("patch", "Patch options:", 70)
	progressFlagGroup       = cli.NewFlagGroup("progress", "Progress options:", 65)
	chartRepoFlagGroup      = cli.NewFlagGroup("chart-repo", "Chart repository options:", 60)
	kubeConnectionFlagGroup = cli.NewFlagGroup("kube-connection", "Kubernetes connection options:", 50)
	performanceFlagGroup    = cli.NewFlagGroup("performance", "Performance options:", 40)
	miscFlagGroup           = cli.NewFlagGroup("misc", "Other options:", 0)
	resourceValidationGroup = cli.NewFlagGroup("resource-local-validation", "Resource local validation options:", 75)
)
