package flag

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/chanced/caps"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

var (
	EnvVarsPrefix string

	definedEnvVarRegexes = make(map[string]*regexp.Regexp)

	_ GetEnvVarRegexesInterface = GetLocalEnvVarRegexes
	_ GetEnvVarRegexesInterface = GetGlobalEnvVarRegexes
	_ GetEnvVarRegexesInterface = GetGlobalAndLocalEnvVarRegexes
	_ GetEnvVarRegexesInterface = GetLocalMultiEnvVarRegexes
	_ GetEnvVarRegexesInterface = GetGlobalMultiEnvVarRegexes
	_ GetEnvVarRegexesInterface = GetGlobalAndLocalMultiEnvVarRegexes
)

type GetEnvVarRegexesInterface func(cmd *cobra.Command, flagName string) ([]string, error)

func GetLocalEnvVarRegexes(cmd *cobra.Command, flagName string) ([]string, error) {
	commandPath := lo.Reverse(strings.SplitN(cmd.CommandPath(), " ", 2))[0]
	expr := "^" + caps.ToScreamingSnake(fmt.Sprintf("%s%s_%s", EnvVarsPrefix, commandPath, flagName)) + "$"

	return []string{expr}, nil
}

func GetLocalMultiEnvVarRegexes(cmd *cobra.Command, flagName string) ([]string, error) {
	commandPath := lo.Reverse(strings.SplitN(cmd.CommandPath(), " ", 2))[0]
	expr := "^" + caps.ToScreamingSnake(fmt.Sprintf("%s%s_%s", EnvVarsPrefix, commandPath, flagName)) + "_.+"

	return []string{expr}, nil
}

func GetGlobalEnvVarRegexes(cmd *cobra.Command, flagName string) ([]string, error) {
	expr := "^" + caps.ToScreamingSnake(fmt.Sprintf("%s%s", EnvVarsPrefix, flagName)) + "$"

	return []string{expr}, nil
}

func GetGlobalMultiEnvVarRegexes(cmd *cobra.Command, flagName string) ([]string, error) {
	expr := "^" + caps.ToScreamingSnake(fmt.Sprintf("%s%s", EnvVarsPrefix, flagName)) + "_.+"

	return []string{expr}, nil
}

func GetGlobalAndLocalEnvVarRegexes(cmd *cobra.Command, flagName string) ([]string, error) {
	globalEnvVarRegexes, err := GetGlobalEnvVarRegexes(cmd, flagName)
	if err != nil {
		return nil, fmt.Errorf("get global env var regexes: %w", err)
	}

	localEnvVarRegexes, err := GetLocalEnvVarRegexes(cmd, flagName)
	if err != nil {
		return nil, fmt.Errorf("get local env var regexes: %w", err)
	}

	return append(globalEnvVarRegexes, localEnvVarRegexes...), nil
}

func GetGlobalAndLocalMultiEnvVarRegexes(cmd *cobra.Command, flagName string) ([]string, error) {
	globalEnvVarRegexes, err := GetGlobalMultiEnvVarRegexes(cmd, flagName)
	if err != nil {
		return nil, fmt.Errorf("get global env var regexes: %w", err)
	}

	localEnvVarRegexes, err := GetLocalMultiEnvVarRegexes(cmd, flagName)
	if err != nil {
		return nil, fmt.Errorf("get local env var regexes: %w", err)
	}

	return append(globalEnvVarRegexes, localEnvVarRegexes...), nil
}

func GetDefinedEnvVarRegexes() map[string]*regexp.Regexp {
	return definedEnvVarRegexes
}

func FindUndefinedEnvVarsInEnviron() []string {
	brandedEnvVars := lo.Filter(os.Environ(), func(envVar string, _ int) bool {
		return strings.HasPrefix(envVar, fmt.Sprintf("%s", EnvVarsPrefix))
	})

	brandedEnvVarNames := lo.Map(brandedEnvVars, func(envVar string, _ int) string {
		envVarName, _, _ := strings.Cut(envVar, "=")
		return envVarName
	})

	var undefinedEnvVars []string
envVarsLoop:
	for _, envVar := range brandedEnvVarNames {
		for _, envVarRegex := range definedEnvVarRegexes {
			if envVarRegex.MatchString(envVar) {
				continue envVarsLoop
			}
		}

		undefinedEnvVars = append(undefinedEnvVars, envVar)
	}

	return undefinedEnvVars
}
