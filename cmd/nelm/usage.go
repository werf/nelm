package main

import (
	"bytes"
	"cmp"
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

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/logboek"
	"github.com/werf/logboek/pkg/types"
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
{{ commandsUsage . | trimTrailingWhitespaces }}
{{- end }}

{{- if .HasAvailableLocalFlags}}
{{ flagsUsage .LocalFlags | trimTrailingWhitespaces }}
{{- end }}
`

var templateFuncs = template.FuncMap{
	"gt": cobra.Gt,
	"trimTrailingWhitespaces": func(s string) string {
		return strings.TrimRightFunc(s, unicode.IsSpace)
	},
	"flagsUsage":    flagsUsage,
	"commandsUsage": commandsUsage,
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
				defValue := strings.TrimPrefix(flag.DefValue, "[")
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

func groupFlags(fset *pflag.FlagSet) ([]cli.FlagGroup, map[cli.FlagGroup][]*pflag.Flag) {
	groupsByPriority := []cli.FlagGroup{}
	groupedFlags := map[cli.FlagGroup][]*pflag.Flag{}

	fset.VisitAll(func(f *pflag.Flag) {
		var group *cli.FlagGroup

		if groupID, found := f.Annotations[cli.FlagGroupIDAnnotationName]; found {
			groupTitle := f.Annotations[cli.FlagGroupTitleAnnotationName]
			groupPriority := f.Annotations[cli.FlagGroupPriorityAnnotationName]
			group = cli.NewFlagGroup(groupID[0], groupTitle[0], lo.Must1(strconv.Atoi(groupPriority[0])))
		} else {
			group = miscFlagGroup
		}

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

func commandsUsage(command *cobra.Command) string {
	if !command.HasAvailableSubCommands() {
		return ""
	}

	subCommands := getSubCommandsRecurse(command)
	groupsByPriority, groupedSubCommandInfos, longestCmdPathLen := groupCmdInfos(subCommands)
	padding := longestCmdPathLen + 3
	cmdIndent := 2

	result := "\n"
	for _, group := range groupsByPriority {
		result += fmt.Sprintf("%s\n", group.Title)
		for _, cmd := range groupedSubCommandInfos[group] {
			result += fmt.Sprintf("%s%s%s\n", strings.Repeat(" ", cmdIndent), fmt.Sprintf("%-*s", padding, cmd.commandPath), cmd.short)
		}
		result += "\n"
	}

	return result
}

func getSubCommandsRecurse(cmd *cobra.Command) []*cobra.Command {
	var allSubCommands []*cobra.Command
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() {
			continue
		}

		if c.HasAvailableSubCommands() {
			allSubCommands = append(allSubCommands, getSubCommandsRecurse(c)...)
		} else {
			allSubCommands = append(allSubCommands, c)
		}
	}

	return allSubCommands
}

func groupCmdInfos(cmds []*cobra.Command) ([]cli.CommandGroup, map[cli.CommandGroup][]*cmdInfo, int) {
	var groupsByPriority []cli.CommandGroup
	var longestCommandPath int
	groupedSubCommandInfos := map[cli.CommandGroup][]*cmdInfo{}

	for _, subCommand := range cmds {
		var group *cli.CommandGroup

		if groupID, found := subCommand.Annotations[cli.CommandGroupIDAnnotationName]; found {
			group = cli.NewCommandGroup(
				groupID,
				subCommand.Annotations[cli.CommandGroupTitleAnnotationName],
				lo.Must(strconv.Atoi(subCommand.Annotations[cli.CommandGroupPriorityAnnotationName])),
			)
		} else {
			group = miscCmdGroup
		}

		commandPath := strings.TrimPrefix(subCommand.CommandPath(), strings.ToLower(common.Brand+" "))

		if len(commandPath) > longestCommandPath {
			longestCommandPath = len(commandPath)
		}

		var commandPriority int
		if priority, found := subCommand.Annotations[cli.CommandPriorityAnnotationName]; found {
			commandPriority = lo.Must(strconv.Atoi(priority))
		} else {
			commandPriority = 10
		}

		groupsByPriority = append(groupsByPriority, *group)
		groupedSubCommandInfos[*group] = append(groupedSubCommandInfos[*group], &cmdInfo{
			commandPath: commandPath,
			priority:    commandPriority,
			short:       subCommand.Short,
		})
	}

	sort.SliceStable(groupsByPriority, func(i, j int) bool {
		return groupsByPriority[i].Priority > groupsByPriority[j].Priority
	})

	groupsByPriority = lo.Uniq(groupsByPriority)

	for group := range groupedSubCommandInfos {
		slices.SortStableFunc(groupedSubCommandInfos[group], func(aInfo, bInfo *cmdInfo) int {
			return cmp.Compare(bInfo.priority, aInfo.priority)
		})
	}

	return groupsByPriority, groupedSubCommandInfos, longestCommandPath
}

type cmdInfo struct {
	commandPath string
	priority    int
	short       string
}
