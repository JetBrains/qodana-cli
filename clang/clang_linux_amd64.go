//go:build linux && amd64

package main

import _ "embed"

//go:generate go run scripts/hash-contents.go clang-linux-x64.tar.gz

//go:embed clang-linux-x64.tar.gz
var Clang []byte

//go:embed clang-linux-x64.sha256.bin
var Hash []byte

var Ext = ".tar.gz"
