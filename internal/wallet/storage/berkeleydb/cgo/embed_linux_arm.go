//go:build cgo && linux && arm && !arm64

package cgo

import _ "embed"

//go:embed libs/linux_arm/libdb-4.8.so
var embeddedLibDB []byte

const embeddedLibName = "libdb-4.8.so"
