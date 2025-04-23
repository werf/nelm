package chart

import (
	"bytes"
	"path"
	"strings"
	"unicode"

	"github.com/werf/3p-helm/pkg/action"
)

type BuildNotesOptions struct {
	RenderSubchartNotes bool
}

func BuildNotes(chartName string, renderedTemplates map[string]string, opts BuildNotesOptions) string {
	var resultBuf bytes.Buffer

	for filePath, fileContent := range renderedTemplates {
		if !strings.HasSuffix(filePath, action.NotesFileSuffix) {
			continue
		}

		fileContent = strings.TrimRightFunc(fileContent, unicode.IsSpace)
		if fileContent == "" {
			continue
		}

		isTopLevelNotes := filePath == path.Join(chartName, "templates", action.NotesFileSuffix)

		if !isTopLevelNotes && !opts.RenderSubchartNotes {
			continue
		}

		if resultBuf.Len() > 0 {
			resultBuf.WriteString("\n")
		}

		resultBuf.WriteString(fileContent)
	}

	return resultBuf.String()
}
