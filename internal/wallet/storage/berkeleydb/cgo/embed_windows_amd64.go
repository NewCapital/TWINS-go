//go:build cgo && windows && amd64

package cgo

// Windows uses statically linked BerkeleyDB via CGO.
// No embedded shared library needed — embeddedLibDB is empty.
var embeddedLibDB []byte

const embeddedLibName = "libdb.dll"
