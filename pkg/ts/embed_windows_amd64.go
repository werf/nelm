//go:build embeddeno

package ts

import _ "embed"

var (
	//go:embed embed/windows/amd64/deno.sha256
	embeddedDenoSHA256 string

	//go:embed embed/windows/amd64/deno.gz
	embeddedDeno []byte
)
