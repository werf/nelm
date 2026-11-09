package spec

// ParsePatchesData parses the contents of a patches file (same format as a chart-shipped
// patches.yaml). Its rules are unscoped, like the ones from LoadPatchesFiles.
func ParsePatchesData(data []byte) (Patches, error) {
	return parsePatchesFile(data)
}
