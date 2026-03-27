package util

import (
	"regexp"
	"strings"
)

var yamlDocSeparator = regexp.MustCompile(`(?m)^---\s*`)

// SplitManifests splits a multi-document YAML string into individual manifest
// strings, filtering out empty documents and documents containing only comments.
// Documents are returned in the order they appear in the input.
func SplitManifests(bigFile string) []string {
	bigFileTmp := strings.TrimSpace(bigFile)
	if bigFileTmp == "" {
		return nil
	}

	docs := yamlDocSeparator.Split(bigFileTmp, -1)

	var result []string
	for _, d := range docs {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}

		hasContent := false
		for _, line := range strings.Split(d, "\n") {
			trimmedLine := strings.TrimSpace(line)
			if trimmedLine != "" && !strings.HasPrefix(trimmedLine, "#") {
				hasContent = true
				break
			}
		}

		if !hasContent {
			continue
		}

		d += "\n"
		result = append(result, d)
	}

	return result
}
