package main

import (
	"bytes"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"unicode"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/werf/logboek"
	"github.com/werf/logboek/pkg/types"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/flag"
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
{{ flagsUsage .LocalFlags | trimTrailingWhitespaces }}
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
	"flagsUsage": flagsUsage,
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
				commandPath: strings.TrimPrefix(cmd.CommandPath(), strings.ToLower(common.Brand+" ")),
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

func flagsUsage(fset *pflag.FlagSet) string {
	const helpIndent = 10
	const minHelpWidthToWrap = 40

	terminalWidth := logboek.Streams().Width()
	groupsByPriority, groupedFlags := groupFlags(fset)

	buf := new(bytes.Buffer)
	lines := []string{}

	for _, group := range groupsByPriority {
		lines = append(lines, fmt.Sprintf("\n%s\n", group.Title))

		for _, flag := range groupedFlags[group] {
			if flag.Hidden {
				continue
			}

			header := ""
			if flag.Shorthand != "" && flag.ShorthandDeprecated == "" {
				header = fmt.Sprintf("  -%s, --%s", flag.Shorthand, flag.Name)
			} else {
				header = fmt.Sprintf("      --%s", flag.Name)
			}

			switch flag.Value.Type() {
			case "string":
				header += fmt.Sprintf("=%q", flag.DefValue)
			case "stringToString":
				defValue := flag.DefValue
				defValue = strings.TrimPrefix(flag.DefValue, "[")
				defValue = strings.TrimSuffix(defValue, "]")
				defValue = fmt.Sprintf("{%s}", defValue)

				header += fmt.Sprintf("=%s", defValue)
			default:
				header += fmt.Sprintf("=%s", flag.DefValue)
			}

			helpWrapWidth := terminalWidth - helpIndent

			var help string
			if helpWrapWidth > minHelpWidthToWrap {
				help = logboek.FitText(flag.Usage, types.FitTextOptions{
					ExtraIndentWidth: helpIndent,
					Width:            helpWrapWidth + helpIndent,
				})
			} else {
				help = fmt.Sprintf("%s%s", strings.Repeat(" ", helpIndent), flag.Usage)
			}

			line := fmt.Sprintf("%s\n%s", header, help)
			lines = append(lines, line)
		}
	}

	for _, line := range lines {
		fmt.Fprintln(buf, line)
	}

	return buf.String()
}

func groupFlags(fset *pflag.FlagSet) ([]flag.Group, map[flag.Group][]*pflag.Flag) {
	groupsByPriority := []flag.Group{}
	groupedFlags := map[flag.Group][]*pflag.Flag{}

	fset.VisitAll(func(f *pflag.Flag) {
		groupID, found := f.Annotations[flag.GroupIDAnnotationName]
		if !found {
			return
		}

		groupTitle := f.Annotations[flag.GroupTitleAnnotationName]
		groupPriority := f.Annotations[flag.GroupPriorityAnnotationName]

		group := flag.NewGroup(groupID[0], groupTitle[0], lo.Must1(strconv.Atoi(groupPriority[0])))

		groupsByPriority = append(groupsByPriority, *group)
		groupedFlags[*group] = append(groupedFlags[*group], f)
	})

	sort.SliceStable(groupsByPriority, func(i, j int) bool {
		return groupsByPriority[i].Priority > groupsByPriority[j].Priority
	})

	groupsByPriority = lo.Uniq(groupsByPriority)

	for group := range groupedFlags {
		slices.SortStableFunc(groupedFlags[group], func(aFlag, bFlag *pflag.Flag) int {
			return strings.Compare(aFlag.Name, bFlag.Name)
		})
	}

	return groupsByPriority, groupedFlags
}

type cmdInfo struct {
	commandPath string
	short       string
}
