package flag

import (
	"fmt"
	"os"
	"time"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

type AddOptions struct {
	GetEnvVarNamesFunc GetEnvVarNamesInterface
	Group              *Group
	Hidden             bool
	Required           bool
	ShortName          string
	Type               Type
}

func Add[T any](
	cmd *cobra.Command,
	dest *T,
	name string,
	defaultValue T,
	help string,
	opts AddOptions,
) error {
	if opts.GetEnvVarNamesFunc == nil {
		opts.GetEnvVarNamesFunc = GetLocalEnvVarName
	}

	envVarNames, err := opts.GetEnvVarNamesFunc(cmd, name)
	if err != nil {
		return fmt.Errorf("get env var names: %w", err)
	}

	for _, envVarName := range envVarNames {
		definedFlagEnvVarNames[envVarName] = struct{}{}
	}

	help = buildHelp(help, envVarNames)

	switch dst := any(dest).(type) {
	case *bool:
		cmd.Flags().BoolVarP(dst, name, opts.ShortName, any(defaultValue).(bool), help)
	case *int:
		cmd.Flags().IntVarP(dst, name, opts.ShortName, any(defaultValue).(int), help)
	case *string:
		cmd.Flags().StringVarP(dst, name, opts.ShortName, any(defaultValue).(string), help)
	case *[]string:
		cmd.Flags().StringSliceVarP(dst, name, opts.ShortName, any(defaultValue).([]string), help)
	case *map[string]string:
		cmd.Flags().StringToStringVarP(dst, name, opts.ShortName, any(defaultValue).(map[string]string), help)
	case *time.Duration:
		cmd.Flags().DurationVarP(dst, name, opts.ShortName, any(defaultValue).(time.Duration), help)
	default:
		panic(fmt.Sprintf("unsupported type %T", dst))
	}

	if opts.Hidden {
		if err := cmd.Flags().MarkHidden(name); err != nil {
			return fmt.Errorf("mark flag as hidden: %w", err)
		}
	}

	if err := processEnvVars(cmd, envVarNames, name); err != nil {
		return fmt.Errorf("process env vars: %w", err)
	}

	if opts.Required {
		if err := cmd.MarkFlagRequired(name); err != nil {
			return fmt.Errorf("mark flag as required: %w", err)
		}
	}

	switch opts.Type {
	case TypeDir:
		if err := cmd.MarkFlagDirname(name); err != nil {
			return fmt.Errorf("mark flag as a directory: %w", err)
		}
	case TypeFile:
		if err := cmd.MarkFlagFilename(name); err != nil {
			return fmt.Errorf("mark flag as a filename: %w", err)
		}
	}

	if opts.Group != nil {
		if err := cmd.Flags().SetAnnotation(name, GroupIDAnnotationName, []string{opts.Group.ID}); err != nil {
			return fmt.Errorf("set group id annotation: %w", err)
		}

		if err := cmd.Flags().SetAnnotation(name, GroupTitleAnnotationName, []string{opts.Group.Title}); err != nil {
			return fmt.Errorf("set group title annotation: %w", err)
		}
	}

	return nil
}

func buildHelp(help string, envVarNames []string) string {
	envVarsHelp := lo.Reduce(envVarNames, func(help string, envVarName string, _ int) string {
		if help == "" {
			return fmt.Sprintf("$%s", envVarName)
		}

		return fmt.Sprintf("%s, $%s", help, envVarName)
	}, "")

	if len(envVarNames) == 0 {
		return help
	} else if len(envVarNames) == 1 {
		return fmt.Sprintf("%s (var %s)", help, envVarsHelp)
	} else {
		return fmt.Sprintf("%s (vars %s)", help, envVarsHelp)
	}
}

func processEnvVars(cmd *cobra.Command, envVarNames []string, flagName string) error {
	lo.Reverse(envVarNames)

	for _, envVarName := range envVarNames {
		value := os.Getenv(envVarName)
		if value == "" {
			continue
		}

		if err := cmd.Flag(flagName).Value.Set(value); err != nil {
			return fmt.Errorf("environment variable %q value %q is not valid: %w", envVarName, value, err)
		}

		break
	}

	return nil
}
