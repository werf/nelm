/*
Copyright The Helm Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package engine

import (
	"fmt"
	"log"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"unicode"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"k8s.io/client-go/rest"

	"github.com/werf/common-go/pkg/util"
	"github.com/werf/nelm/pkg/helm/pkg/chart"
	"github.com/werf/nelm/pkg/helm/pkg/chartutil"
	"github.com/werf/nelm/pkg/helm/pkg/werf/helmopts"
	"github.com/werf/nelm/pkg/helm/pkg/werf/secrets/gotmplfunctions"
	"github.com/werf/nelm/pkg/helm/pkg/werf/secrets/runtimedata"
)

// Engine is an implementation of the Helm rendering implementation for templates.
type Engine struct {
	// If strict is enabled, template rendering will fail if a template references
	// a value that was not passed in.
	Strict bool
	// In LintMode, some 'required' template values may be missing, so don't fail
	LintMode bool
	// optional provider of clients to talk to the Kubernetes API
	clientProvider *ClientProvider
	// EnableDNS tells the engine to allow DNS lookups when rendering templates
	EnableDNS bool
}

// New creates a new instance of Engine using the passed in rest config.
func New(config *rest.Config) Engine {
	var clientProvider ClientProvider = clientProviderFromConfig{config}
	return Engine{
		clientProvider: &clientProvider,
	}
}

// Render takes a chart, optional values, and value overrides, and attempts to render the Go templates.
//
// Render can be called repeatedly on the same engine.
//
// This will look in the chart's 'templates' data (e.g. the 'templates/' directory)
// and attempt to render the templates there using the values passed in.
//
// Values are scoped to their templates. A dependency template will not have
// access to the values set for its parent. If chart "foo" includes chart "bar",
// "bar" will not have access to the values for "foo".
//
// Values should be prepared with something like `chartutils.ReadValues`.
//
// Values are passed through the templates according to scope. If the top layer
// chart includes the chart foo, which includes the chart bar, the values map
// will be examined for a table called "foo". If "foo" is found in vals,
// that section of the values will be passed into the "foo" chart. And if that
// section contains a value named "bar", that value will be passed on to the
// bar chart during render time.
func (e Engine) Render(chrt *chart.Chart, values chartutil.Values, opts helmopts.HelmOptions) (map[string]string, error) {
	tmap := allTemplates(chrt, values, opts)
	return e.render(tmap, chrt.SecretsRuntimeData, opts)
}

// Render takes a chart, optional values, and value overrides, and attempts to
// render the Go templates using the default options.
func Render(chrt *chart.Chart, values chartutil.Values, opts helmopts.HelmOptions) (map[string]string, error) {
	return new(Engine).Render(chrt, values, opts)
}

// RenderWithClient takes a chart, optional values, and value overrides, and attempts to
// render the Go templates using the default options. This engine is client aware and so can have template
// functions that interact with the client.
func RenderWithClient(chrt *chart.Chart, values chartutil.Values, config *rest.Config, opts helmopts.HelmOptions) (map[string]string, error) {
	var clientProvider ClientProvider = clientProviderFromConfig{config}
	return Engine{
		clientProvider: &clientProvider,
	}.Render(chrt, values, opts)
}

// RenderWithClientProvider takes a chart, optional values, and value overrides, and attempts to
// render the Go templates using the default options. This engine is client aware and so can have template
// functions that interact with the client.
// This function differs from RenderWithClient in that it lets you customize the way a dynamic client is constructed.
func RenderWithClientProvider(chrt *chart.Chart, values chartutil.Values, clientProvider ClientProvider, opts helmopts.HelmOptions) (map[string]string, error) {
	return Engine{
		clientProvider: &clientProvider,
	}.Render(chrt, values, opts)
}

// renderable is an object that can be rendered.
type renderable struct {
	// tpl is the current template.
	tpl string
	// vals are the values to be supplied to the template.
	vals chartutil.Values
	// namespace prefix to the templates of the current chart
	basePath string
}

const warnStartDelim = "HELM_ERR_START"
const warnEndDelim = "HELM_ERR_END"
const recursionMaxNums = 1000

var warnRegex = regexp.MustCompile(warnStartDelim + `((?s).*)` + warnEndDelim)

func warnWrap(warn string) string {
	return warnStartDelim + warn + warnEndDelim
}

// 'include' needs to be defined in the scope of a 'tpl' template as
// well as regular file-loaded templates.
func includeFun(t *template.Template, includedNames map[string]int) func(string, interface{}) (string, error) {
	return func(name string, data interface{}) (string, error) {
		var buf strings.Builder
		if v, ok := includedNames[name]; ok {
			if v > recursionMaxNums {
				return "", errors.Wrapf(fmt.Errorf("unable to execute template"), "rendering template has a nested reference name: %s", name)
			}
			includedNames[name]++
		} else {
			includedNames[name] = 1
		}
		err := t.ExecuteTemplate(&buf, name, data)
		includedNames[name]--
		if err != nil {
			return "", detailedTemplateError(t, detailedTemplateErrorData{
				funcName:     "include",
				templateName: name,
			}, Debug, err)
		}

		return buf.String(), nil
	}
}

// As does 'tpl', so that nested calls to 'tpl' see the templates
// defined by their enclosing contexts.
func tplFun(parent *template.Template, includedNames map[string]int, strict bool) func(string, interface{}) (string, error) {
	return func(tpl string, vals interface{}) (string, error) {
		// No templating required if plain text with no templates passed.
		if !strings.Contains(tpl, "{{") && !strings.Contains(tpl, "}}") {
			return tpl, nil
		}

		t, err := parent.Clone()
		if err != nil {
			return "", errors.Wrapf(err, "cannot clone template")
		}

		// Re-inject the missingkey option, see text/template issue https://github.com/golang/go/issues/43022
		// We have to go by strict from our engine configuration, as the option fields are private in Template.
		// TODO: Remove workaround (and the strict parameter) once we build only with golang versions with a fix.
		if strict {
			t.Option("missingkey=error")
		} else {
			t.Option("missingkey=zero")
		}

		// Re-inject 'include' so that it can close over our clone of t;
		// this lets any 'define's inside tpl be 'include'd.
		t.Funcs(template.FuncMap{
			"include": includeFun(t, includedNames),
			"tpl":     tplFun(t, includedNames, strict),

			"include_debug": includeDebugFun(t, includedNames),
			"tpl_debug":     tplDebugFun(t, includedNames, strict),
		})

		// We need a .New template, as template text which is just blanks
		// or comments after parsing out defines just addes new named
		// template definitions without changing the main template.
		// https://pkg.go.dev/text/template#Template.Parse
		// Use the parent's name for lack of a better way to identify the tpl
		// text string. (Maybe we could use a hash appended to the name?)
		t, err = t.New(parent.Name()).Parse(tpl)
		if err != nil {
			return "", detailedTemplateError(t, detailedTemplateErrorData{
				funcName:        "tpl",
				templateName:    parent.Name(),
				templateContent: tpl,
			}, Debug, err)
		}

		var buf strings.Builder
		if err := t.Execute(&buf, vals); err != nil {
			return "", detailedTemplateError(t, detailedTemplateErrorData{
				funcName:        "tpl",
				templateName:    parent.Name(),
				templateContent: tpl,
			}, Debug, err)
		}

		// See comment in renderWithReferences explaining the <no value> hack.
		return strings.ReplaceAll(buf.String(), "<no value>", ""), nil
	}
}

// initFunMap creates the Engine's FuncMap and adds context-specific functions.
func (e Engine) initFunMap(t *template.Template, secretsRuntimeData runtimedata.RuntimeData, opts helmopts.HelmOptions) {
	funcMap := funcMap()
	includedNames := make(map[string]int)

	// Add the template-rendering functions here so we can close over t.
	funcMap["include"] = includeFun(t, includedNames)
	funcMap["tpl"] = tplFun(t, includedNames, e.Strict)

	funcMap["include_debug"] = includeDebugFun(t, includedNames)
	funcMap["tpl_debug"] = tplDebugFun(t, includedNames, e.Strict)

	// Add the `required` function here so we can use lintMode
	funcMap["required"] = func(warn string, val interface{}) (interface{}, error) {
		if val == nil {
			if e.LintMode {
				// Don't fail on missing required values when linting
				log.Printf("[INFO] Missing required value: %s", warn)
				return "", nil
			}
			return val, errors.Errorf(warnWrap(warn))
		} else if _, ok := val.(string); ok {
			if val == "" {
				if e.LintMode {
					// Don't fail on missing required values when linting
					log.Printf("[INFO] Missing required value: %s", warn)
					return "", nil
				}
				return val, errors.Errorf(warnWrap(warn))
			}
		}
		return val, nil
	}

	// Override sprig fail function for linting and wrapping message
	funcMap["fail"] = func(msg string) (string, error) {
		if e.LintMode {
			// Don't fail when linting
			log.Printf("[INFO] Fail: %s", msg)
			return "", nil
		}
		return "", errors.New(warnWrap(msg))
	}

	// If we are not linting and have a cluster connection, provide a Kubernetes-backed
	// implementation.
	if !e.LintMode && e.clientProvider != nil {
		funcMap["lookup"] = newLookupFunction(*e.clientProvider)
	}

	// When DNS lookups are not enabled override the sprig function and return
	// an empty string.
	if !e.EnableDNS {
		funcMap["getHostByName"] = func(name string) string {
			return ""
		}
	}

	funcMap["printf_debug"] = func(format string, args ...interface{}) string {
		if Debug {
			log.Printf("-- printf_debug format %q result:\n%s\n\n", format, fmt.Sprintf(format, args...))
		}

		return ""
	}

	funcMap["dump_debug"] = func(obj interface{}) string {
		if Debug {
			log.Printf("-- dump_debug result:\n%s\n\n", spew.Sdump(obj))
		}

		return ""
	}

	switch opts.ChartLoadOpts.ChartType {
	case helmopts.ChartTypeBundle, helmopts.ChartTypeChart, helmopts.ChartTypeChartStub:
		gotmplfunctions.SetupWerfSecretFile(secretsRuntimeData, funcMap)
	case helmopts.ChartTypeSubchart:
	default:
		panic("unknown extender type")
	}

	t.Funcs(funcMap)
}

// render takes a map of templates/values and renders them.
func (e Engine) render(tpls map[string]renderable, secretsRuntimeData runtimedata.RuntimeData, opts helmopts.HelmOptions) (rendered map[string]string, err error) {
	// Basically, what we do here is start with an empty parent template and then
	// build up a list of templates -- one for each file. Once all of the templates
	// have been parsed, we loop through again and execute every template.
	//
	// The idea with this process is to make it possible for more complex templates
	// to share common blocks, but to make the entire thing feel like a file-based
	// template engine.
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("rendering template failed: %v", r)
		}
	}()
	t := template.New("gotpl")
	if e.Strict {
		t.Option("missingkey=error")
	} else {
		// Not that zero will attempt to add default values for types it knows,
		// but will still emit <no value> for others. We mitigate that later.
		t.Option("missingkey=zero")
	}

	e.initFunMap(t, secretsRuntimeData, opts)

	// We want to parse the templates in a predictable order. The order favors
	// higher-level (in file system) templates over deeply nested templates.
	keys := sortTemplates(tpls)

	for _, filename := range keys {
		r := tpls[filename]
		if _, err := t.New(filename).Parse(r.tpl); err != nil {
			return map[string]string{}, detailedTemplateError(t, detailedTemplateErrorData{
				templateName:    filename,
				templateContent: r.tpl,
			}, Debug, cleanupParseError(filename, err))
		}
	}

	rendered = make(map[string]string, len(keys))
	for _, filename := range keys {
		// Don't render partials. We don't care out the direct output of partials.
		// They are only included from other templates.
		if strings.HasPrefix(path.Base(filename), "_") {
			continue
		}
		// At render time, add information about the template that is being rendered.
		vals := tpls[filename].vals
		vals["Template"] = chartutil.Values{"Name": filename, "BasePath": tpls[filename].basePath}
		var buf strings.Builder
		if err := t.ExecuteTemplate(&buf, filename, vals); err != nil {
			return map[string]string{}, detailedTemplateError(t, detailedTemplateErrorData{
				templateName:    filename,
				templateContent: tpls[filename].tpl,
			}, Debug, cleanupExecError(filename, err))
		}

		// Work around the issue where Go will emit "<no value>" even if Options(missing=zero)
		// is set. Since missing=error will never get here, we do not need to handle
		// the Strict case.
		rendered[filename] = strings.ReplaceAll(buf.String(), "<no value>", "")
	}

	return rendered, nil
}

func cleanupParseError(filename string, err error) error {
	tokens := strings.Split(err.Error(), ": ")
	if len(tokens) == 1 {
		// This might happen if a non-templating error occurs
		return fmt.Errorf("parse error in (%s): %s", filename, err)
	}
	// The first token is "template"
	// The second token is either "filename:lineno" or "filename:lineNo:columnNo"
	location := tokens[1]
	// The remaining tokens make up a stacktrace-like chain, ending with the relevant error
	errMsg := tokens[len(tokens)-1]
	return fmt.Errorf("parse error at (%s): %s", string(location), errMsg)
}

func cleanupExecError(filename string, err error) error {
	if _, isExecError := err.(template.ExecError); !isExecError {
		return err
	}

	tokens := strings.SplitN(err.Error(), ": ", 3)
	if len(tokens) != 3 {
		// This might happen if a non-templating error occurs
		return fmt.Errorf("execution error in (%s): %s", filename, err)
	}

	// The first token is "template"
	// The second token is either "filename:lineno" or "filename:lineNo:columnNo"
	location := tokens[1]

	parts := warnRegex.FindStringSubmatch(tokens[2])
	if len(parts) >= 2 {
		return fmt.Errorf("execution error at (%s): %s", string(location), parts[1])
	}

	return err
}

func sortTemplates(tpls map[string]renderable) []string {
	keys := make([]string, len(tpls))
	i := 0
	for key := range tpls {
		keys[i] = key
		i++
	}
	sort.Sort(sort.Reverse(byPathLen(keys)))
	return keys
}

type byPathLen []string

func (p byPathLen) Len() int      { return len(p) }
func (p byPathLen) Swap(i, j int) { p[j], p[i] = p[i], p[j] }
func (p byPathLen) Less(i, j int) bool {
	a, b := p[i], p[j]
	ca, cb := strings.Count(a, "/"), strings.Count(b, "/")
	if ca == cb {
		return strings.Compare(a, b) == -1
	}
	return ca < cb
}

// allTemplates returns all templates for a chart and its dependencies.
//
// As it goes, it also prepares the values in a scope-sensitive manner.
func allTemplates(c *chart.Chart, vals chartutil.Values, opts helmopts.HelmOptions) map[string]renderable {
	templates := make(map[string]renderable)
	recAllTpls(c, templates, vals, opts)
	return templates
}

// recAllTpls recurses through the templates in a chart.
//
// As it recurses, it also sets the values to be appropriate for the template
// scope.
func recAllTpls(c *chart.Chart, templates map[string]renderable, vals chartutil.Values, opts helmopts.HelmOptions) map[string]interface{} {
	subCharts := make(map[string]interface{})
	chartMetaData := struct {
		chart.Metadata
		IsRoot bool
	}{*c.Metadata, c.IsRoot()}

	next := map[string]interface{}{
		"Chart":        chartMetaData,
		"Files":        newFiles(c.Files),
		"Release":      vals["Release"],
		"Capabilities": vals["Capabilities"],
		"Values":       make(chartutil.Values),
		"Subcharts":    subCharts,
		"Runtime":      vals["Runtime"],
	}

	next = lo.Assign(opts.ChartLoadOpts.DefaultRootContext, next)

	// If there is a {{.Values.ThisChart}} in the parent metadata,
	// copy that into the {{.Values}} for this template.
	if c.IsRoot() {
		next["Values"] = vals["Values"]
	} else if vs, err := vals.Table("Values." + c.Name()); err == nil {
		next["Values"] = vs
	}

	for _, child := range c.Dependencies() {
		subCharts[child.Name()] = recAllTpls(child, templates, next, opts)
	}

	newParentID := c.ChartFullPath()
	for _, t := range c.Templates {
		if t == nil {
			continue
		}
		if !isTemplateValid(c, t.Name) {
			continue
		}
		templates[path.Join(newParentID, t.Name)] = renderable{
			tpl:      string(t.Data),
			vals:     next,
			basePath: path.Join(newParentID, "templates"),
		}
	}

	return next
}

// isTemplateValid returns true if the template is valid for the chart type
func isTemplateValid(ch *chart.Chart, templateName string) bool {
	if isLibraryChart(ch) {
		return strings.HasPrefix(filepath.Base(templateName), "_")
	}
	return true
}

// isLibraryChart returns true if the chart is a library chart
func isLibraryChart(c *chart.Chart) bool {
	return strings.EqualFold(c.Metadata.Type, "library")
}

func detailedTemplateError(tmpl *template.Template, d detailedTemplateErrorData, debug bool, err error) error {
	if debug {
		if d.templateContent == "" {
			d.templateContent, _ = templateContentFromTree(tmpl, d.templateName)
		}

		var funcNameMsg string
		if d.funcName != "" {
			funcNameMsg = fmt.Sprintf("  Function name: %q\n", d.funcName)
		}

		return fmt.Errorf(
			"%w\n\nDetails:\n%s  Template name: %q\n  Template content:\n%s",
			err,
			funcNameMsg,
			d.templateName,
			strings.TrimRightFunc(util.NumerateLines(d.templateContent, 1), unicode.IsSpace),
		)
	}

	if strings.Contains(err.Error(), TemplateErrHint) {
		return err
	}

	return fmt.Errorf("%w\n%s", err, TemplateErrHint)
}

type detailedTemplateErrorData struct {
	funcName        string
	templateName    string
	templateContent string
}

func templateContentFromTree(tmpl *template.Template, name string) (string, error) {
	t := tmpl.Lookup(name)
	if t == nil || t.Tree == nil || t.Tree.Root == nil {
		return "", fmt.Errorf("template %q not found", name)
	}

	return strings.TrimSpace(t.Tree.Root.String()), nil
}

func includeDebugFun(t *template.Template, includedNames map[string]int) func(string, interface{}) (string, error) {
	return func(name string, data interface{}) (string, error) {
		var buf strings.Builder
		if v, ok := includedNames[name]; ok {
			if v > recursionMaxNums {
				return "", errors.Wrapf(fmt.Errorf("unable to execute template"), "rendering template has a nested reference name: %s", name)
			}
			includedNames[name]++
		} else {
			includedNames[name] = 1
		}
		execErr := t.ExecuteTemplate(&buf, name, data)
		includedNames[name]--

		var templateContent string
		if execErr != nil || Debug {
			templateContent, _ = templateContentFromTree(t, name)
		}

		if Debug {
			log.Printf("-- include_debug template %q content:\n%s\n\n", name, templateContent)
		}

		if execErr != nil {
			return "", detailedTemplateError(t, detailedTemplateErrorData{
				funcName:        "include_debug",
				templateName:    name,
				templateContent: templateContent,
			}, Debug, execErr)
		}

		if Debug {
			log.Printf("-- include_debug template %q result:\n%s\n\n", name, buf.String())
		}

		return buf.String(), nil
	}
}

func tplDebugFun(parent *template.Template, includedNames map[string]int, strict bool) func(string, interface{}) (string, error) {
	return func(tpl string, vals interface{}) (string, error) {
		// No templating required if plain text with no templates passed.
		if !strings.Contains(tpl, "{{") && !strings.Contains(tpl, "}}") {
			return tpl, nil
		}

		t, err := parent.Clone()
		if err != nil {
			return "", errors.Wrapf(err, "cannot clone template")
		}

		// Re-inject the missingkey option, see text/template issue https://github.com/golang/go/issues/43022
		// We have to go by strict from our engine configuration, as the option fields are private in Template.
		// TODO: Remove workaround (and the strict parameter) once we build only with golang versions with a fix.
		if strict {
			t.Option("missingkey=error")
		} else {
			t.Option("missingkey=zero")
		}

		// Re-inject 'include' so that it can close over our clone of t;
		// this lets any 'define's inside tpl be 'include'd.
		t.Funcs(template.FuncMap{
			"include": includeFun(t, includedNames),
			"tpl":     tplFun(t, includedNames, strict),

			"include_debug": includeDebugFun(t, includedNames),
			"tpl_debug":     tplDebugFun(t, includedNames, strict),
		})

		// We need a .New template, as template text which is just blanks
		// or comments after parsing out defines just addes new named
		// template definitions without changing the main template.
		// https://pkg.go.dev/text/template#Template.Parse
		// Use the parent's name for lack of a better way to identify the tpl
		// text string. (Maybe we could use a hash appended to the name?)
		t, err = t.New(parent.Name()).Parse(tpl)
		if err != nil {
			return "", detailedTemplateError(t, detailedTemplateErrorData{
				funcName:        "tpl_debug",
				templateName:    parent.Name(),
				templateContent: tpl,
			}, Debug, err)
		}

		if Debug {
			log.Printf("-- tpl_debug %q content:\n%s\n\n", parent.Name(), tpl)
		}

		var buf strings.Builder
		if err := t.Execute(&buf, vals); err != nil {
			return "", detailedTemplateError(t, detailedTemplateErrorData{
				funcName:        "tpl_debug",
				templateName:    parent.Name(),
				templateContent: tpl,
			}, Debug, err)
		}

		// See comment in renderWithReferences explaining the <no value> hack.
		result := strings.ReplaceAll(buf.String(), "<no value>", "")

		if Debug {
			log.Printf("-- tpl_debug %q result:\n%s\n\n", parent.Name(), result)
		}

		return result, nil
	}
}

var (
	TemplateErrHint = `Set log level to "debug" to get more details about this error.`
	Debug           bool
)
