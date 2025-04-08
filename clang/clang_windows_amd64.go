//go:build windows && amd64

package main

import _ "embed"

//go:generate go run scripts/hash-contents.go clang-win-x64.zip

//go:embed clang-win-x64.zip
var Clang []byte

//go:embed clang-win-x64.sha256.bin
var Hash []byte

var Ext = ".zip"
