//go:build darwin && arm64

package main

import _ "embed"

//go:embed clang-mac-aarch64.tar.gz
var Clang []byte
var Ext = ".tar.gz"
