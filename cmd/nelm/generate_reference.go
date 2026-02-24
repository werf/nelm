package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/featgate"
)

type generateReferenceConfig struct{}

func runGenerateReference(cmd *cobra.Command, args []string) error {
	rootCmd := cmd.Root()
	markdown := generateReferenceDoc(rootCmd)

	outPath := filepath.Join("docs", "reference.md")

	if err := os.MkdirAll("docs", 0o755); err != nil {
		return fmt.Errorf("creating docs directory: %w", err)
	}

	if err := os.WriteFile(outPath, []byte(markdown), 0o644); err != nil {
		return fmt.Errorf("writing reference.md: %w", err)
	}

	fmt.Printf("Generated %s\n", outPath)

	return nil
}

func generateReferenceDoc(rootCmd *cobra.Command) string {
	var buf bytes.Buffer

	subCommands := getSubCommandsRecurse(rootCmd)
	groupsByPriority, groupedSubCommandInfos, _ := groupCmdInfos(subCommands)

	cmdMap := make(map[string]*cobra.Command)
	for _, cmd := range subCommands {
		commandPath := strings.TrimPrefix(cmd.CommandPath(), strings.ToLower(common.Brand+" "))
		cmdMap[commandPath] = cmd
	}

	buf.WriteString(renderCommandsOverview(groupsByPriority, groupedSubCommandInfos))

	buf.WriteString("## Commands\n\n")

	for _, group := range groupsByPriority {
		for _, cmdInfo := range groupedSubCommandInfos[group] {
			if cmd, ok := cmdMap[cmdInfo.commandPath]; ok {
				buf.WriteString(renderCommandMarkdown(cmd, cmdInfo.commandPath))
			}
		}
	}

	buf.WriteString(renderFeatGatesMarkdown())

	return buf.String()
}

func renderCommandMarkdown(cmd *cobra.Command, commandPath string) string {
	var buf bytes.Buffer

	buf.WriteString(fmt.Sprintf("### %s\n\n", commandPath))

	if cmd.Long != "" {
		buf.WriteString(util.EscapeForMarkdownPreservingCodeSpans(cmd.Long) + "\n\n")
	} else if cmd.Short != "" {
		buf.WriteString(util.EscapeForMarkdownPreservingCodeSpans(cmd.Short) + "\n\n")
	}

	buf.WriteString("**Usage:**\n\n")
	buf.WriteString(fmt.Sprintf("```shell\n%s\n```\n\n", cmd.UseLine()))

	if len(cmd.Aliases) > 0 {
		buf.WriteString("**Aliases:** ")
		buf.WriteString(fmt.Sprintf("`%s`\n\n", cmd.NameAndAliases()))
	}

	if cmd.Example != "" {
		buf.WriteString("**Examples:**\n\n")
		buf.WriteString(fmt.Sprintf("```shell\n%s\n```\n\n", cmd.Example))
	}

	if cmd.HasAvailableLocalFlags() {
		buf.WriteString(renderFlagsMarkdown(cmd.LocalFlags()))
	}

	return buf.String()
}

func renderFlagsMarkdown(fset *pflag.FlagSet) string {
	groupsByPriority, groupedFlags := groupFlags(fset)

	var buf bytes.Buffer

	for _, group := range groupsByPriority {
		hasVisibleFlags := false
		for _, flag := range groupedFlags[group] {
			if !flag.Hidden {
				hasVisibleFlags = true
				break
			}
		}

		if !hasVisibleFlags {
			continue
		}

		buf.WriteString(fmt.Sprintf("**%s**\n\n", group.Title))

		for _, flag := range groupedFlags[group] {
			if flag.Hidden {
				continue
			}

			buf.WriteString(renderFlagMarkdown(flag))
		}

		buf.WriteString("\n")
	}

	return buf.String()
}

func renderCommandsOverview(groupsByPriority []cli.CommandGroup, groupedSubCommandInfos map[cli.CommandGroup][]*cmdInfo) string {
	if len(groupsByPriority) == 0 {
		return ""
	}

	var buf bytes.Buffer
	buf.WriteString("## Commands Overview\n\n")

	for _, group := range groupsByPriority {
		buf.WriteString(fmt.Sprintf("### %s\n\n", strings.TrimSuffix(group.Title, ":")))

		for _, cmdInfo := range groupedSubCommandInfos[group] {
			buf.WriteString(fmt.Sprintf("- [`%s %s`](#%s) — %s\n", strings.ToLower(common.Brand), cmdInfo.commandPath, commandPathToAnchor(cmdInfo.commandPath), util.EscapeForMarkdownPreservingCodeSpans(cmdInfo.short)))
		}

		buf.WriteString("\n")
	}

	return buf.String()
}

func renderFlagMarkdown(flag *pflag.Flag) string {
	var buf bytes.Buffer

	var flagName string
	if flag.Shorthand != "" && flag.ShorthandDeprecated == "" {
		flagName = fmt.Sprintf("`-%s`, `--%s`", flag.Shorthand, flag.Name)
	} else {
		flagName = fmt.Sprintf("`--%s`", flag.Name)
	}

	var defValue string
	switch flag.Value.Type() {
	case "string":
		defValue = fmt.Sprintf(`"%s"`, normalizePathForDocs(flag.DefValue))
	case "stringToString":
		v := strings.TrimPrefix(flag.DefValue, "[")
		v = strings.TrimSuffix(v, "]")
		defValue = fmt.Sprintf("{%s}", v)
	default:
		defValue = flag.DefValue
	}

	buf.WriteString(fmt.Sprintf("- %s (default: `%s`)\n\n", flagName, defValue))
	buf.WriteString(fmt.Sprintf("  %s\n\n", util.EscapeForMarkdownPreservingCodeSpans(flag.Usage)))

	return buf.String()
}

func commandPathToAnchor(commandPath string) string {
	return strings.ToLower(strings.ReplaceAll(commandPath, " ", "-"))
}

func newGenerateReferenceCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	_ = &generateReferenceConfig{}

	cmd := cli.NewSubCommand(
		ctx,
		"generate-reference",
		"Generate CLI reference documentation.",
		"Generate CLI reference documentation.",
		0,
		miscCmdGroup,
		cli.SubCommandOptions{},
		runGenerateReference,
	)

	cmd.Hidden = true

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		return nil
	}

	return cmd
}

func normalizePathForDocs(path string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	if suffix, found := strings.CutPrefix(path, homeDir); found {
		return "~" + suffix
	}

	return path
}

func renderFeatGatesMarkdown() string {
	if len(featgate.FeatGates) == 0 {
		return ""
	}

	var buf bytes.Buffer
	buf.WriteString("## Feature Gates\n\n")
	buf.WriteString("Feature gates are experimental features that can be enabled via environment variables.\n\n")

	for _, fg := range featgate.FeatGates {
		buf.WriteString(fmt.Sprintf("### %s\n\n", fg.EnvVarName()))
		buf.WriteString(fmt.Sprintf("**Default:** `%v`\n\n", fg.Default()))
		buf.WriteString(fmt.Sprintf("%s\n\n", util.EscapeForMarkdownPreservingCodeSpans(fg.Help)))
	}

	return buf.String()
}
