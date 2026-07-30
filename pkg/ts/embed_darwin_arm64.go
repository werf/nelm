//go:build embeddeno

package ts

import _ "embed"

var (
	//go:embed embed/darwin/arm64/deno.sha256
	embeddedDenoSHA256 string

	//go:embed embed/darwin/arm64/deno.gz
	embeddedDeno []byte
)
