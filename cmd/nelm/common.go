package main

import (
	"strings"

	"github.com/samber/lo"
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
	modes := lo.Map(action.LogColorModes, func(mode action.LogColorMode, _ int) string {
		return string(mode)
	})

	return "Allowed: " + strings.Join(modes, ", ")
}

func allowedLogLevelsHelp() string {
	levels := lo.Map(log.Levels, func(level log.Level, _ int) string {
		return string(level)
	})

	return "Allowed: " + strings.Join(levels, ", ")
}
