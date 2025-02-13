package main

import (
	"context"

	"github.com/spf13/cobra"
)

func BuildRootCommand(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "nelm",
		Long:          "Nelm is a Helm 3 replacement. Nelm manages and deploys Helm Charts to Kubernetes just like Helm, but provides a lot of features, improvements and bug fixes on top of what Helm 3 offers.",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

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

const usageTemplate = `Usage:
{{- if .Runnable}}
  {{.UseLine}}
{{- end}}

{{- if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]
{{- end}}

{{- if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}
{{- end}}

{{- if .HasExample}}

Examples:
{{.Example}}
{{- end}}

{{- if .HasAvailableSubCommands}}{{$cmds := .Commands}}
  {{- if eq (len .Groups) 0}}

Available commands:
    {{- range $cmds}}
      {{- if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}
      {{- end}}
    {{- end}}
  {{- else}}
    {{- range $group := .Groups}}

{{.Title}}
      {{- range $cmds}}
        {{- if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}
        {{- end}}
      {{- end}}
    {{- end}}

    {{- if not .AllChildCommandsHaveGroup}}

Additional commands:
      {{- range $cmds}}
        {{- if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}
        {{- end}}
      {{- end}}
    {{- end}}
  {{- end}}
{{- end}}

{{- if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}
{{- end}}

{{- if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}
{{- end}}

{{- if .HasHelpSubCommands}}

Additional help topics:
  {{- range .Commands}}
    {{- if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}
    {{- end}}
  {{- end}}
{{- end}}

{{- if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.
{{- end}}
`
