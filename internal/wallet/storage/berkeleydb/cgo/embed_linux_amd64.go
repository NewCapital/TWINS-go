//go:build cgo && linux && amd64

package cgo

import _ "embed"

//go:embed libs/linux_amd64/libdb-4.8.so
var embeddedLibDB []byte

const embeddedLibName = "libdb-4.8.so"
