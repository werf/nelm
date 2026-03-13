package helm

var (
	Settings = settings

	LoadReleasesInMemory = loadReleasesInMemory
	Debug                = debug
	NewRootCmd           = newRootCmd

	NewDependencyCmd = newDependencyCmd
	NewHistoryCmd    = newHistoryCmd
	NewListCmd       = newListCmd
	NewRepoCmd       = newRepoCmd
	NewPackageCmd    = newPackageCmd
	NewSearchCmd     = newSearchCmd
	NewRegistryCmd   = newRegistryCmd
	NewPullCmd       = newPullCmd
	NewPushCmd       = newPushCmd

	LoadPlugins = loadPlugins
)

func IsPluginError(err error) bool {
	if err != nil {
		_, isPluginErr := err.(pluginError)
		return isPluginErr
	}
	return false
}

func PluginErrorCode(err error) int {
	if pluginErr, ok := err.(pluginError); ok {
		return pluginErr.code
	}
	return 0
}
