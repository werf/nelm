//go:build embednode

package ts

import _ "embed"

//go:embed embed/linux/amd64/node.gz
var embeddedNode []byte

//go:embed embed/linux/amd64/npm.tar.gz
var embeddedNPM []byte
