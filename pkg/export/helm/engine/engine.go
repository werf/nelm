package engine

import internal "github.com/werf/nelm/pkg/helm/pkg/engine"

func GetDebug() bool {
	return internal.Debug
}

func GetTemplateErrHint() string {
	return internal.TemplateErrHint
}

func SetDebug(v bool) {
	internal.Debug = v
}

func SetTemplateErrHint(v string) {
	internal.TemplateErrHint = v
}
