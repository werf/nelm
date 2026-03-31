package helm

import internal "github.com/werf/nelm/pkg/helm/cmd/helm"

var Settings = internal.Settings

func IsPluginError(err error) bool {
	return internal.IsPluginError(err)
}

func PluginErrorCode(err error) int {
	return internal.PluginErrorCode(err)
}
