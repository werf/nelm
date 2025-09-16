package main

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/log"
)

const (
	releaseNameStub      = "release-stub"
	releaseNamespaceStub = "namespace-stub"
)

var helmRootCmd *cobra.Command

func allowedLogColorModesHelp() string {
	return "Allowed: " + strings.Join(log.LogColorModes, ", ")
}

func allowedLogLevelsHelp() string {
	return "Allowed: " + strings.Join(action.LogLevels, ", ")
}
