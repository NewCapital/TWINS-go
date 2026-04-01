//go:build cgo && darwin && amd64

package cgo

import _ "embed"

//go:embed libs/darwin_amd64/libdb-4.8.dylib
var embeddedLibDB []byte

const embeddedLibName = "libdb-4.8.dylib"
