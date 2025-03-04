package action

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/samber/lo"
	"sigs.k8s.io/yaml"

	"github.com/werf/3p-helm/pkg/chart/loader"
	"github.com/werf/3p-helm/pkg/werf/secrets"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

const DefaultVersionOutputFormat = common.YamlOutputFormat

type VersionOptions struct {
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
		result.MajorVersion = lo.ToPtr(int(semVer.Major()))
		result.MinorVersion = lo.ToPtr(int(semVer.Minor()))
		result.PatchVersion = lo.ToPtr(int(semVer.Patch()))
	}

	if !opts.OutputNoPrint {
		var resultMessage string

		switch opts.OutputFormat {
		case common.JsonOutputFormat:
			b, err := json.MarshalIndent(result, "", strings.Repeat(" ", 4))
			if err != nil {
				return nil, fmt.Errorf("marshal result to json: %w", err)
			}

			resultMessage = string(b)
		case common.YamlOutputFormat:
			b, err := yaml.Marshal(result)
			if err != nil {
				return nil, fmt.Errorf("marshal result to yaml: %w", err)
			}

			resultMessage = string(b)
		default:
			return nil, fmt.Errorf("unknown output format %q", opts.OutputFormat)
		}

		fmt.Print(resultMessage)
	}

	return result, nil
}

func applyVersionOptionsDefaults(opts VersionOptions) (VersionOptions, error) {
	if opts.OutputFormat == "" {
		opts.OutputFormat = DefaultVersionOutputFormat
	}

	return opts, nil
}

type VersionResult struct {
	FullVersion  string `json:"full"`
	MajorVersion *int   `json:"major,omitempty"`
	MinorVersion *int   `json:"minor,omitempty"`
	PatchVersion *int   `json:"patch,omitempty"`
}
