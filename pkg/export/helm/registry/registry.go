package registry

import internal "github.com/werf/nelm/internal/helm/pkg/registry"

type Client = internal.Client
type ClientOption = internal.ClientOption

var (
	NewClient                = internal.NewClient
	ClientOptCredentialsFile = internal.ClientOptCredentialsFile
	ClientOptDebug           = internal.ClientOptDebug
	ClientOptPlainHTTP       = internal.ClientOptPlainHTTP
	ClientOptWriter          = internal.ClientOptWriter
)
