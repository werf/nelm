package main

import (
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	"github.com/werf/nelm/pkg/log"
)

const IgnoreBundleJSFlagDescription = "Do not use the existing bundle.js file. Requires TypeScript source files and Deno to rebuild."

var helmRootCmd *cobra.Command

func allowedLogColorModesHelp() string {
	return "Allowed: " + strings.Join(log.LogColorModes, ", ")
}

func allowedLogLevelsHelp() string {
	return "Allowed: " + strings.Join(lo.Map(log.Levels, func(lvl log.Level, _ int) string {
		return string(lvl)
	}), ", ")
}
