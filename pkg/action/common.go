package action

type LogColorMode string

const (
	LogColorModeDefault LogColorMode = ""
	LogColorModeOff     LogColorMode = "off"
	LogColorModeOn      LogColorMode = "on"
)

type ReleaseStorageDriver string

const (
	ReleaseStorageDriverDefault    ReleaseStorageDriver = ""
	ReleaseStorageDriverSecrets    ReleaseStorageDriver = "secrets"
	ReleaseStorageDriverSecret     ReleaseStorageDriver = "secret"
	ReleaseStorageDriverConfigMaps ReleaseStorageDriver = "configmaps"
	ReleaseStorageDriverConfigMap  ReleaseStorageDriver = "configmap"
	ReleaseStorageDriverMemory     ReleaseStorageDriver = "memory"
	ReleaseStorageDriverSQL        ReleaseStorageDriver = "sql"
)
