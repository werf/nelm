//go:build embeddeno

package ts

import _ "embed"

//go:embed embed/darwin/arm64/deno.gz
var embeddedDeno []byte
