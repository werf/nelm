package action

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/goccy/go-yaml"
	"github.com/gookit/color"

	"github.com/werf/3p-helm/pkg/chart/loader"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultVersionLogLevel     = log.ErrorLevel
	DefaultVersionOutputFormat = common.OutputFormatYAML
)

type VersionOptions struct {
	// OutputFormat specifies the output format for version information.
	// Valid values: "yaml" (default), "json".
	// Defaults to DefaultVersionOutputFormat (yaml) if not specified.
	OutputFormat string
	// OutputNoPrint, when true, suppresses printing the output and only returns the result data structure.
	// Useful when calling this programmatically.
	OutputNoPrint bool
	// TempDirPath is the directory for temporary files during the operation.
	// A temporary directory is created automatically if not specified.
	TempDirPath string
}

type VersionResult struct {
	FullVersion  string `json:"full"`
	MajorVersion int    `json:"major"`
	MinorVersion int    `json:"minor"`
	PatchVersion int    `json:"patch"`
}

func Version(ctx context.Context, opts VersionOptions) (*VersionResult, error) {
	opts, err := applyVersionOptionsDefaults(opts)
	if err != nil {
		return nil, fmt.Errorf("build version options: %w", err)
	}

	loader.NoChartLockWarning = ""

	result := &VersionResult{
		FullVersion: common.Version,
	}

	if semVer, err := semver.StrictNewVersion(common.Version); err == nil {
		result.MajorVersion = util.Uint64ToInt(semVer.Major())
		result.MinorVersion = util.Uint64ToInt(semVer.Minor())
		result.PatchVersion = util.Uint64ToInt(semVer.Patch())
	}

	if !opts.OutputNoPrint {
		var resultMessage string

		switch opts.OutputFormat {
		case common.OutputFormatJSON:
			b, err := json.MarshalIndent(result, "", strings.Repeat(" ", 2))
			if err != nil {
				return nil, fmt.Errorf("marshal result to json: %w", err)
			}

			resultMessage = string(b)
		case common.OutputFormatYAML:
			b, err := yaml.MarshalContext(ctx, result, yaml.UseLiteralStyleIfMultiline(true))
			if err != nil {
				return nil, fmt.Errorf("marshal result to yaml: %w", err)
			}

			resultMessage = string(b)
		default:
			return nil, fmt.Errorf("unknown output format %q", opts.OutputFormat)
		}

		var colorLevel color.Level
		if color.Enable {
			colorLevel = color.TermColorLevel()
		}

		if err := writeWithSyntaxHighlight(os.Stdout, resultMessage, opts.OutputFormat, colorLevel); err != nil {
			return nil, fmt.Errorf("write result to output: %w", err)
		}
	}

	return result, nil
}

func applyVersionOptionsDefaults(opts VersionOptions) (VersionOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return VersionOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if opts.OutputFormat == "" {
		opts.OutputFormat = DefaultVersionOutputFormat
	}

	return opts, nil
}
