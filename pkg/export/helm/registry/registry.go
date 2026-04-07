package registry

import internal "github.com/werf/nelm/pkg/helm/pkg/registry"

var (
	NewClient                = internal.NewClient
	ClientOptCredentialsFile = internal.ClientOptCredentialsFile
	ClientOptDebug           = internal.ClientOptDebug
	ClientOptPlainHTTP       = internal.ClientOptPlainHTTP
	ClientOptWriter          = internal.ClientOptWriter
)

type Client = internal.Client

type ClientOption = internal.ClientOption
