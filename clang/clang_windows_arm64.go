//go:build windows && arm64

package main

import _ "embed"

//go:generate go run scripts/hash-contents.go clang-win-aarch64.zip

//go:embed clang-win-aarch64.zip
var Clang []byte

//go:embed clang-win-aarch64.sha256.bin
var Hash []byte

var Ext = ".zip"
