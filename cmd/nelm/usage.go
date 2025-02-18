package main

import (
	"fmt"
	"strings"
	"text/template"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/werf/nelm/pkg/common"
)

const helpTemplate = `
{{- with (or .Long .Short)}}
{{- . | trimTrailingWhitespaces}}
{{- end}}

{{- if or .Runnable .HasSubCommands}}
{{- .UsageString}}
{{- end }}
`

const usageTemplate = `
{{- if (and .Runnable .HasParent) }}

Usage:
  {{.UseLine}}
{{- end}}

{{- if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}
{{- end}}

{{- if .HasExample}}

Examples:
{{.Example}}
{{- end}}

{{- if .HasAvailableSubCommands}}
  {{- if eq (len .Groups) 0}}

Commands:
    {{- range cmdsShorts (list .) }}
  {{.}}
    {{- end }}
  {{- else}}
    {{- range $group := .Groups}}

{{.Title}}
      {{- $groupedCmds := list }}
      {{- range $cmd := $.Commands}}
        {{- if (and (eq $cmd.GroupID $group.ID) $cmd.IsAvailableCommand)}}
          {{- $groupedCmds = append $groupedCmds $cmd}}
        {{- end}}
      {{- end}}

      {{- range cmdsShorts $groupedCmds}}
  {{.}}
      {{- end }}
    {{- end}}

    {{- if not .AllChildCommandsHaveGroup}}
      {{- $ungroupedCmds := list }}
      {{- range $cmd := .Commands}}
        {{- if (and (eq $cmd.GroupID "") $cmd.IsAvailableCommand)}}
          {{- $ungroupedCmds = append $ungroupedCmds $cmd}}
        {{- end}}
      {{- end}}

Additional commands:
      {{- range cmdsShorts $ungroupedCmds }}
  {{.}}
      {{- end }}
    {{- end}}
  {{- end}}
{{- end}}

{{- if .HasAvailableLocalFlags}}

Options:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces }}
{{- end }}
`

var templateFuncs = template.FuncMap{
	"gt":         cobra.Gt,
	"eq":         cobra.Eq,
	"cmdsShorts": cmdsShorts,
	"append":     common.SprigFuncs["append"],
	"list":       common.SprigFuncs["list"],
	"trimTrailingWhitespaces": func(s string) string {
		return strings.TrimRightFunc(s, unicode.IsSpace)
	},
}

func usageFunc(c *cobra.Command) error {
	t := template.New("top")
	t.Funcs(templateFuncs)

	if _, err := t.Parse(c.UsageTemplate()); err != nil {
		c.PrintErrln(err)
		return err
	}

	if err := t.Execute(c.OutOrStderr(), c); err != nil {
		c.PrintErrln(err)
		return err
	}

	return nil
}

func cmdsShorts(commands []any) []string {
	var cmds []*cobra.Command
	for _, cmd := range commands {
		c, ok := cmd.(*cobra.Command)
		if !ok {
			panic(fmt.Sprintf("unexpected type %T", cmd))
		}

		cmds = append(cmds, c)
	}

	var infos []*cmdInfo
	for _, cmd := range cmds {
		infos = append(infos, cmdInfosRecurse(cmd)...)
	}

	padding := longestCommandPathLength(infos) + 3

	var result []string
	for _, info := range infos {
		result = append(result, fmt.Sprintf("%s%s", fmt.Sprintf("%-*s", padding, info.commandPath), info.short))
	}

	return result
}

func cmdInfosRecurse(cmd *cobra.Command) []*cmdInfo {
	if !cmd.HasAvailableSubCommands() {
		return []*cmdInfo{
			&cmdInfo{
				commandPath: cmd.CommandPath(),
				short:       cmd.Short,
			},
		}
	}

	var infos []*cmdInfo
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() {
			continue
		}

		infos = append(infos, cmdInfosRecurse(c)...)
	}

	return infos
}

func longestCommandPathLength(infos []*cmdInfo) int {
	var longest int
	for _, info := range infos {
		if len(info.commandPath) > longest {
			longest = len(info.commandPath)
		}
	}

	return longest
}

type cmdInfo struct {
	commandPath string
	short       string
}
