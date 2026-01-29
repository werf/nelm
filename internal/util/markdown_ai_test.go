//go:build ai_tests

package util_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/werf/nelm/internal/util"
)

func TestAI_EscapeForMarkdown(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		assert.Equal(t, "", util.EscapeForMarkdown(""))
	})

	t.Run("plain text unchanged", func(t *testing.T) {
		assert.Equal(t, "Hello World", util.EscapeForMarkdown("Hello World"))
	})

	t.Run("backslash", func(t *testing.T) {
		assert.Equal(t, `path\\to\\file`, util.EscapeForMarkdown(`path\to\file`))
	})

	t.Run("backticks", func(t *testing.T) {
		assert.Equal(t, "use \\`code\\` here", util.EscapeForMarkdown("use `code` here"))
	})

	t.Run("asterisks", func(t *testing.T) {
		assert.Equal(t, `\*bold\* and \*\*bolder\*\*`, util.EscapeForMarkdown("*bold* and **bolder**"))
	})

	t.Run("underscores", func(t *testing.T) {
		assert.Equal(t, `\_italic\_ and \_\_bold\_\_`, util.EscapeForMarkdown("_italic_ and __bold__"))
	})

	t.Run("env vars", func(t *testing.T) {
		assert.Equal(t, `\$NELM\_NAMESPACE`, util.EscapeForMarkdown("$NELM_NAMESPACE"))
	})

	t.Run("env vars with wildcard", func(t *testing.T) {
		assert.Equal(t,
			`Vars: \$NELM\_LABELS\_\*, \$NELM\_RELEASE\_INSTALL\_LABELS\_\*`,
			util.EscapeForMarkdown("Vars: $NELM_LABELS_*, $NELM_RELEASE_INSTALL_LABELS_*"))
	})

	t.Run("curly braces", func(t *testing.T) {
		assert.Equal(t, `template: \{\{ \.Values \}\}`, util.EscapeForMarkdown("template: {{ .Values }}"))
	})

	t.Run("square brackets", func(t *testing.T) {
		assert.Equal(t, `\[link text\]\(url\)`, util.EscapeForMarkdown("[link text](url)"))
	})

	t.Run("angle brackets", func(t *testing.T) {
		assert.Equal(t, `\<html\> tags`, util.EscapeForMarkdown("<html> tags"))
	})

	t.Run("headings", func(t *testing.T) {
		assert.Equal(t, `\# heading`, util.EscapeForMarkdown("# heading"))
	})

	t.Run("unordered list plus", func(t *testing.T) {
		assert.Equal(t, `\+ item`, util.EscapeForMarkdown("+ item"))
	})

	t.Run("unordered list minus", func(t *testing.T) {
		assert.Equal(t, `\- item`, util.EscapeForMarkdown("- item"))
	})

	t.Run("ordered list", func(t *testing.T) {
		assert.Equal(t, `1\. item`, util.EscapeForMarkdown("1. item"))
	})

	t.Run("images", func(t *testing.T) {
		assert.Equal(t, `\!\[alt\]\(image\.png\)`, util.EscapeForMarkdown("![alt](image.png)"))
	})

	t.Run("tables", func(t *testing.T) {
		assert.Equal(t, `col1 \| col2`, util.EscapeForMarkdown("col1 | col2"))
	})

	t.Run("strikethrough", func(t *testing.T) {
		assert.Equal(t, `\~\~strikethrough\~\~`, util.EscapeForMarkdown("~~strikethrough~~"))
	})

	t.Run("math mode", func(t *testing.T) {
		assert.Equal(t, `\$x^2\$`, util.EscapeForMarkdown("$x^2$"))
	})

	t.Run("CLI usage line", func(t *testing.T) {
		assert.Equal(t,
			`nelm release install \[options\.\.\.\] \-n namespace \-r release \[chart\-dir\]`,
			util.EscapeForMarkdown("nelm release install [options...] -n namespace -r release [chart-dir]"))
	})

	t.Run("flag description", func(t *testing.T) {
		result := util.EscapeForMarkdown("Add labels to all resources. Vars: $NELM_LABELS_*, $NELM_RELEASE_INSTALL_LABELS_*")
		assert.Contains(t, result, `\$NELM\_LABELS\_\*`)
	})

	t.Run("complex CLI output", func(t *testing.T) {
		assert.Equal(t, `\-\-stringToString=\{key=value\}`, util.EscapeForMarkdown(`--stringToString={key=value}`))
	})

	t.Run("file path", func(t *testing.T) {
		assert.Equal(t, `/home/user/\.docker/config\.json`, util.EscapeForMarkdown("/home/user/.docker/config.json"))
	})

	t.Run("duration unchanged", func(t *testing.T) {
		assert.Equal(t, "5s", util.EscapeForMarkdown("5s"))
	})

	t.Run("boolean unchanged", func(t *testing.T) {
		assert.Equal(t, "false", util.EscapeForMarkdown("false"))
	})

	t.Run("multiple special chars", func(t *testing.T) {
		assert.Equal(t, `\*\_\[text\]\_\* with \$var and \\path`, util.EscapeForMarkdown(`*_[text]_* with $var and \path`))
	})

	t.Run("all special chars", func(t *testing.T) {
		assert.Equal(t,
			"\\\\\\`\\*\\_\\{\\}\\[\\]\\<\\>\\(\\)\\#\\+\\-\\.\\!\\|\\~\\$",
			util.EscapeForMarkdown("\\`*_{}[]<>()#+-.!|~$"))
	})

	t.Run("consecutive special chars", func(t *testing.T) {
		assert.Equal(t, `\*\*\*`, util.EscapeForMarkdown("***"))
	})

	t.Run("unicode preserved", func(t *testing.T) {
		assert.Equal(t, `text with \*emphasis\*`, util.EscapeForMarkdown("text with *emphasis*"))
	})

	t.Run("newlines preserved", func(t *testing.T) {
		assert.Equal(t, "line1\nline2\n\\*bold\\*", util.EscapeForMarkdown("line1\nline2\n*bold*"))
	})

	t.Run("tabs preserved", func(t *testing.T) {
		assert.Equal(t, "col1\tcol2\t\\*bold\\*", util.EscapeForMarkdown("col1\tcol2\t*bold*"))
	})
}

func TestAI_EscapeForMarkdownPreservingCodeSpans(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		assert.Equal(t, "", util.EscapeForMarkdownPreservingCodeSpans(""))
	})

	t.Run("plain text unchanged", func(t *testing.T) {
		assert.Equal(t, "Hello World", util.EscapeForMarkdownPreservingCodeSpans("Hello World"))
	})

	t.Run("inline code preserved", func(t *testing.T) {
		assert.Equal(t, "use `$NELM_VAR` here", util.EscapeForMarkdownPreservingCodeSpans("use `$NELM_VAR` here"))
	})

	t.Run("inline code with surrounding text", func(t *testing.T) {
		assert.Equal(t,
			`\*bold\* and `+"`code_here`"+` and \_italic\_`,
			util.EscapeForMarkdownPreservingCodeSpans("*bold* and `code_here` and _italic_"))
	})

	t.Run("triple backticks inline", func(t *testing.T) {
		assert.Equal(t, "before ```$VAR_NAME``` after", util.EscapeForMarkdownPreservingCodeSpans("before ```$VAR_NAME``` after"))
	})

	t.Run("code block preserved", func(t *testing.T) {
		input := "text *bold*\n```\n$VAR=value\n_underscore_\n```\nmore *text*"
		result := util.EscapeForMarkdownPreservingCodeSpans(input)
		assert.Contains(t, result, "```\n$VAR=value\n_underscore_\n```")
		assert.Contains(t, result, `\*bold\*`)
		assert.Contains(t, result, `\*text\*`)
	})

	t.Run("unmatched backtick escaped", func(t *testing.T) {
		assert.Equal(t, "text with \\` unmatched", util.EscapeForMarkdownPreservingCodeSpans("text with ` unmatched"))
	})

	t.Run("unmatched triple backtick", func(t *testing.T) {
		assert.Equal(t, "text with ``\\` unmatched", util.EscapeForMarkdownPreservingCodeSpans("text with ``` unmatched"))
	})

	t.Run("multiple code spans", func(t *testing.T) {
		assert.Equal(t, "`code1` and \\*bold\\* and `code2`", util.EscapeForMarkdownPreservingCodeSpans("`code1` and *bold* and `code2`"))
	})

	t.Run("nested patterns in code", func(t *testing.T) {
		assert.Equal(t, "Use `--flag=*` for wildcards", util.EscapeForMarkdownPreservingCodeSpans("Use `--flag=*` for wildcards"))
	})

	t.Run("env var in code", func(t *testing.T) {
		assert.Equal(t,
			"Set `$NELM_NAMESPACE` to override\\. \\$OTHER\\_VAR is also used\\.",
			util.EscapeForMarkdownPreservingCodeSpans("Set `$NELM_NAMESPACE` to override. $OTHER_VAR is also used."))
	})

	t.Run("empty code span", func(t *testing.T) {
		assert.Equal(t, "empty `` code", util.EscapeForMarkdownPreservingCodeSpans("empty `` code"))
	})

	t.Run("backslash in code", func(t *testing.T) {
		assert.Equal(t, "path `C:\\Users\\name` here", util.EscapeForMarkdownPreservingCodeSpans("path `C:\\Users\\name` here"))
	})

	t.Run("indented code block preserved", func(t *testing.T) {
		input := "normal *text*\n    $ helm list --filter 'ara[a-z]+'\n    NAME    CHART\nmore *text*"
		result := util.EscapeForMarkdownPreservingCodeSpans(input)
		assert.Contains(t, result, "    $ helm list --filter 'ara[a-z]+'")
		assert.Contains(t, result, "    NAME    CHART")
		assert.Contains(t, result, `normal \*text\*`)
		assert.Contains(t, result, `more \*text\*`)
	})

	t.Run("tab indented code block preserved", func(t *testing.T) {
		input := "normal *text*\n\t$ command --flag\nmore *text*"
		result := util.EscapeForMarkdownPreservingCodeSpans(input)
		assert.Contains(t, result, "\t$ command --flag")
		assert.Contains(t, result, `normal \*text\*`)
	})

	t.Run("mixed code formats", func(t *testing.T) {
		input := "Use `--flag` option:\n    $ helm list --filter '*'\nOr use `--all`."
		result := util.EscapeForMarkdownPreservingCodeSpans(input)
		assert.Contains(t, result, "`--flag`")
		assert.Contains(t, result, "`--all`")
		assert.Contains(t, result, "    $ helm list --filter '*'")
		assert.Contains(t, result, "option:")
	})

	t.Run("helm list long description", func(t *testing.T) {
		input := `If the --filter flag is provided, it will be treated as a filter. Filters are
regular expressions (Perl compatible) that are applied to the list of releases.
Only items that match the filter will be returned.

    $ helm list --filter 'ara[a-z]+'
    NAME                UPDATED                                  CHART
    maudlin-arachnid    2020-06-18 14:17:46.125134977 +0000 UTC  alpine-0.1.0

If no results are found, 'helm list' will exit 0, but with no output (or in
the case of no '-q' flag, only headers).`

		result := util.EscapeForMarkdownPreservingCodeSpans(input)
		assert.Contains(t, result, "    $ helm list --filter 'ara[a-z]+'")
		assert.Contains(t, result, "    maudlin-arachnid")
		assert.Contains(t, result, `\-\-filter`)
		assert.Contains(t, result, "'helm list'")
	})
}
