package engine

import internal "github.com/werf/nelm/internal/helm/pkg/engine"

func SetDebug(v bool)             { internal.Debug = v }
func GetDebug() bool              { return internal.Debug }
func SetTemplateErrHint(v string) { internal.TemplateErrHint = v }
func GetTemplateErrHint() string  { return internal.TemplateErrHint }
