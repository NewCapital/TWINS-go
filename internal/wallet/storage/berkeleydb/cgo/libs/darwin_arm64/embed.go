//go:build darwin && arm64

package libs

import _ "embed"

//go:embed libdb-4.8.dylib
var LibDB []byte

const LibName = "libdb-4.8.dylib"
