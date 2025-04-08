//go:build darwin && amd64

package main

import _ "embed"

//go:generate go run scripts/hash-contents.go clang-mac-x64.tar.gz

//go:embed clang-mac-x64.tar.gz
var Clang []byte

//go:embed clang-mac-x64.sha256.bin
var Hash []byte

var Ext = ".tar.gz"
