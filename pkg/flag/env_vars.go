package flag

import (
	"fmt"
	"os"
	"strings"

	"github.com/chanced/caps"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

var (
	EnvVarsPrefix string

	definedFlagEnvVarNames = make(map[string]struct{})

	_ GetEnvVarNamesInterface = GetLocalEnvVarName
	_ GetEnvVarNamesInterface = GetGlobalEnvVarName
	_ GetEnvVarNamesInterface = GetGlobalAndLocalEnvVarName
)

type GetEnvVarNamesInterface func(cmd *cobra.Command, flagName string) ([]string, error)

func GetLocalEnvVarName(cmd *cobra.Command, flagName string) ([]string, error) {
	return []string{caps.ToScreamingSnake(fmt.Sprintf("%s_%s", cmd.CommandPath(), flagName))}, nil
}

func GetGlobalEnvVarName(cmd *cobra.Command, flagName string) ([]string, error) {
	return []string{caps.ToScreamingSnake(fmt.Sprintf("%s%s", EnvVarsPrefix, flagName))}, nil
}

func GetGlobalAndLocalEnvVarName(cmd *cobra.Command, flagName string) ([]string, error) {
	globalEnvVar, err := GetGlobalEnvVarName(cmd, flagName)
	if err != nil {
		return nil, fmt.Errorf("get global env var name: %w", err)
	}

	localEnvVar, err := GetLocalEnvVarName(cmd, flagName)
	if err != nil {
		return nil, fmt.Errorf("get local env var name: %w", err)
	}

	return append(globalEnvVar, localEnvVar...), nil
}

func GetDefinedFlagEnvVarNames() []string {
	var envVarNames []string

	for envVarName := range definedFlagEnvVarNames {
		envVarNames = append(envVarNames, envVarName)
	}

	return envVarNames
}

func FindUndefinedFlagEnvVarsInEnviron() []string {
	brandedEnvVars := lo.Filter(os.Environ(), func(envVar string, _ int) bool {
		return strings.HasPrefix(envVar, fmt.Sprintf("%s", EnvVarsPrefix))
	})

	brandedEnvVarNames := lo.Map(brandedEnvVars, func(envVar string, _ int) string {
		envVarName, _, _ := strings.Cut(envVar, "=")
		return envVarName
	})

	var undefinedEnvVars []string
	for _, envVar := range brandedEnvVarNames {
		if _, ok := definedFlagEnvVarNames[envVar]; ok {
			continue
		}

		undefinedEnvVars = append(undefinedEnvVars, envVar)
	}

	return undefinedEnvVars
}
