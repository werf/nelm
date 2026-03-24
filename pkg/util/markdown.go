package util

import (
	"slices"
	"strings"
)

var markdownSpecialChars = []string{
	`\`, "`", "*", "_", "{", "}", "[", "]", "<", ">",
	"(", ")", "#", "+", "-", ".", "!", "|", "~", "$",
}

// EscapeForMarkdown escapes all Markdown special characters in the input string
// by prefixing them with a backslash.
func EscapeForMarkdown(s string) string {
	if s == "" {
		return s
	}

	result := s
	for _, char := range markdownSpecialChars {
		result = strings.ReplaceAll(result, char, `\`+char)
	}

	return result
}

// EscapeForMarkdownPreservingCodeSpans escapes Markdown special characters
// but preserves text within backtick code spans and indented code blocks.
func EscapeForMarkdownPreservingCodeSpans(s string) string {
	if s == "" {
		return s
	}

	lines := strings.Split(s, "\n")

	var resultLines []string

	inFencedBlock := false

	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "```") {
			inFencedBlock = !inFencedBlock

			resultLines = append(resultLines, line)

			continue
		}

		if inFencedBlock {
			resultLines = append(resultLines, line)

			continue
		}

		if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			resultLines = append(resultLines, line)

			continue
		}

		resultLines = append(resultLines, escapeLinePreservingInlineCode(line))
	}

	return strings.Join(resultLines, "\n")
}

func escapeLinePreservingInlineCode(line string) string {
	var result strings.Builder

	result.Grow(len(line) * 2)

	i := 0
	for i < len(line) {
		if line[i] == '`' {
			end := strings.Index(line[i+1:], "`")
			if end != -1 {
				result.WriteString(line[i : i+1+end+1])
				i = i + 1 + end + 1

				continue
			}
		}

		char := string(line[i])
		if isMarkdownSpecialChar(char) {
			result.WriteString(`\`)
		}

		result.WriteString(char)

		i++
	}

	return result.String()
}

func isMarkdownSpecialChar(char string) bool {
	return slices.Contains(markdownSpecialChars, char)
}
