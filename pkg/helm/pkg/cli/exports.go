package cli

import "k8s.io/cli-runtime/pkg/genericclioptions"

var EnvOr = envOr

func (s *EnvSettings) GetNamespaceP() *string {
	return &s.namespace
}

func (s *EnvSettings) GetConfigP() *genericclioptions.RESTClientGetter {
	return &s.config
}
