//go:build embeddeno

package ts

import _ "embed"

//go:embed embed/darwin/amd64/deno.gz
var embeddedDeno []byte
