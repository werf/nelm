package flag

import (
	"encoding/csv"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type AddOptions struct {
	GetEnvVarRegexesFunc GetEnvVarRegexesInterface
	Group                *Group
	Hidden               bool
	Required             bool
	ShortName            string
	Type                 Type
}

// TODO(ilya-lesikov): human-readable form for var regexes in usage
// TODO(ilya-lesikov): allow restricted values
// TODO(ilya-lesikov): show restricted values in usage
// TODO(ilya-lesikov): pass examples separately?
// TODO(ilya-lesikov): allow for []string with no comma-separated values (pflag.StringArrayVar?)
// TODO(ilya-lesikov): allow for map[string]string with no comma-separated values
// TODO(ilya-lesikov): refactor into AddScalar, AddSlice and AddMap? Or some other structure, check what pflags can already handle
// TODO(ilya-lesikov): document
// TODO(ilya-lesikov): unit tests
func Add[T any](cmd *cobra.Command, dest *T, name string, defaultValue T, help string, opts AddOptions) error {
	opts, err := applyAddOptionsDefaults(opts, dest)
	if err != nil {
		return fmt.Errorf("apply defaults: %w", err)
	}

	envVarRegexExprs, err := opts.GetEnvVarRegexesFunc(cmd, name)
	if err != nil {
		return fmt.Errorf("get env var names: %w", err)
	}

	help, err = buildHelp(help, dest, envVarRegexExprs)
	if err != nil {
		return fmt.Errorf("build help: %w", err)
	}

	if err := addFlags(cmd, dest, name, opts.ShortName, defaultValue, help); err != nil {
		return fmt.Errorf("add flags: %w", err)
	}

	if opts.Hidden {
		if err := cmd.Flags().MarkHidden(name); err != nil {
			return fmt.Errorf("mark flag as hidden: %w", err)
		}
	}

	if err := processEnvVars(cmd, envVarRegexExprs, name, dest); err != nil {
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

func applyAddOptionsDefaults[T any](opts AddOptions, dest *T) (AddOptions, error) {
	if opts.GetEnvVarRegexesFunc == nil {
		switch dst := any(dest).(type) {
		case *bool, *int, *string, *time.Duration:
			opts.GetEnvVarRegexesFunc = GetLocalEnvVarRegexes
		case *[]string, *map[string]string:
			opts.GetEnvVarRegexesFunc = GetLocalMultiEnvVarRegexes
		default:
			return AddOptions{}, fmt.Errorf("unsupported type %T", dst)
		}
	}

	return opts, nil
}

func buildHelp[T any](help string, dest *T, envVarRegexes []string) (string, error) {
	if len(envVarRegexes) == 0 {
		return help, nil
	} else if len(envVarRegexes) == 1 {
		help = fmt.Sprintf("%s (var %s)", help, envVarRegexes[0])
	} else {
		help = fmt.Sprintf("%s (vars %s)", help, strings.Join(envVarRegexes, ", "))
	}

	switch dst := any(dest).(type) {
	case *bool, *int, *string, *time.Duration:
	case *[]string, *map[string]string:
		help += " (can specify multiple times)"
	default:
		return "", fmt.Errorf("unsupported type %T", dst)
	}

	return help, nil
}

func addFlags[T any](cmd *cobra.Command, dest *T, name string, shortName string, defaultValue T, help string) error {
	switch dst := any(dest).(type) {
	case *bool:
		cmd.Flags().BoolVarP(dst, name, shortName, any(defaultValue).(bool), help)
	case *int:
		cmd.Flags().IntVarP(dst, name, shortName, any(defaultValue).(int), help)
	case *string:
		cmd.Flags().StringVarP(dst, name, shortName, any(defaultValue).(string), help)
	case *[]string:
		cmd.Flags().StringSliceVarP(dst, name, shortName, any(defaultValue).([]string), help)
	case *map[string]string:
		cmd.Flags().StringToStringVarP(dst, name, shortName, any(defaultValue).(map[string]string), help)
	case *time.Duration:
		cmd.Flags().DurationVarP(dst, name, shortName, any(defaultValue).(time.Duration), help)
	default:
		return fmt.Errorf("unsupported type %T", dst)
	}

	return nil
}

func processEnvVars[T any](cmd *cobra.Command, envVarRegexExprs []string, flagName string, dest T) error {
	for _, expr := range envVarRegexExprs {
		regex, err := regexp.Compile(fmt.Sprintf(`%s`, expr))
		if err != nil {
			return fmt.Errorf("compile regex %q: %w", expr, err)
		}

		definedEnvVarRegexes[expr] = regex
	}

	lo.Reverse(envVarRegexExprs)

	environ := os.Environ()
	sort.Strings(environ)

	envir := map[string]string{}
	for _, keyValue := range environ {
		parts := strings.SplitN(keyValue, "=", 2)
		envir[parts[0]] = parts[1]
	}

	envs := map[string]string{}
	for key, val := range envir {
		for _, regexExpr := range envVarRegexExprs {
			if !definedEnvVarRegexes[regexExpr].MatchString(key) || val == "" {
				continue
			}

			envs[key] = val
		}
	}

	switch dst := any(dest).(type) {
	case *bool, *int, *string, *time.Duration:
	envirLoop:
		for key, val := range envir {
			for _, regexExpr := range envVarRegexExprs {
				if !definedEnvVarRegexes[regexExpr].MatchString(key) || val == "" {
					continue
				}

				if err := cmd.Flag(flagName).Value.Set(val); err != nil {
					return fmt.Errorf("environment variable %q value %q is not valid: %w", key, val, err)
				}

				break envirLoop
			}
		}
	case *[]string:
		for key, val := range envs {
			parts, err := splitComma(val)
			if err != nil {
				return fmt.Errorf("split comma-separated environment variable %q with value %q: %w", key, val, err)
			}

			for _, part := range parts {
				if err := cmd.Flag(flagName).Value.(pflag.SliceValue).Append(part); err != nil {
					return fmt.Errorf("environment variable %q value %q is not valid: %w", key, val, err)
				}
			}
		}
	case *map[string]string:
		for key, val := range envs {
			if err := cmd.Flag(flagName).Value.Set(val); err != nil {
				return fmt.Errorf("environment variable %q value %q is not valid: %w", key, val, err)
			}
		}
	default:
		return fmt.Errorf("unsupported type %T", dst)
	}

	return nil
}

func splitComma(s string) ([]string, error) {
	stringReader := strings.NewReader(s)
	csvReader := csv.NewReader(stringReader)

	parts, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("read csv values: %w", err)
	}

	return parts, nil
}
