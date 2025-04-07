//go:build windows && arm64

package main

import _ "embed"

//go:embed clang-win-aarch64.zip
var Clang []byte
var Ext = ".zip"
