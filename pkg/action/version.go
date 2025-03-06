package action

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/goccy/go-yaml"
	color "github.com/gookit/color"

	"github.com/werf/3p-helm/pkg/chart/loader"
	"github.com/werf/3p-helm/pkg/werf/secrets"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

const DefaultVersionOutputFormat = common.YamlOutputFormat

type VersionOptions struct {
	LogColorMode  LogColorMode
	OutputFormat  common.OutputFormat
	OutputNoPrint bool
}

func Version(ctx context.Context, opts VersionOptions) (*VersionResult, error) {
	log.Default.SetLevel(ctx, log.SilentLevel)

	opts, err := applyVersionOptionsDefaults(opts)
	if err != nil {
		return nil, fmt.Errorf("build version options: %w", err)
	}

	secrets.DisableSecrets = true
	loader.NoChartLockWarning = ""

	result := &VersionResult{
		FullVersion: common.Version,
	}

	if semVer, err := semver.StrictNewVersion(common.Version); err == nil {
		result.MajorVersion = int(semVer.Major())
		result.MinorVersion = int(semVer.Minor())
		result.PatchVersion = int(semVer.Patch())
	}

	if !opts.OutputNoPrint {
		var resultMessage string

		switch opts.OutputFormat {
		case common.JsonOutputFormat:
			b, err := json.MarshalIndent(result, "", strings.Repeat(" ", 2))
			if err != nil {
				return nil, fmt.Errorf("marshal result to json: %w", err)
			}

			resultMessage = string(b)
		case common.YamlOutputFormat:
			b, err := yaml.MarshalContext(ctx, result)
			if err != nil {
				return nil, fmt.Errorf("marshal result to yaml: %w", err)
			}

			resultMessage = string(b)
		default:
			return nil, fmt.Errorf("unknown output format %q", opts.OutputFormat)
		}

		var colorLevel color.Level
		if opts.LogColorMode != LogColorModeOff {
			colorLevel = color.DetectColorLevel()
		}

		if err := writeWithSyntaxHighlight(os.Stdout, resultMessage, string(opts.OutputFormat), colorLevel); err != nil {
			return nil, fmt.Errorf("write result to output: %w", err)
		}
	}

	return result, nil
}

func applyVersionOptionsDefaults(opts VersionOptions) (VersionOptions, error) {
	if opts.OutputFormat == "" {
		opts.OutputFormat = DefaultVersionOutputFormat
	}

	opts.LogColorMode = applyLogColorModeDefault(opts.LogColorMode, false)

	return opts, nil
}

type VersionResult struct {
	FullVersion  string `json:"full"`
	MajorVersion int    `json:"major"`
	MinorVersion int    `json:"minor"`
	PatchVersion int    `json:"patch"`
}
