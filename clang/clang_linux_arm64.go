//go:build linux && arm64

package main

import _ "embed"

//go:embed clang-linux-aarch64.tar.gz
var Clang []byte
var Ext = ".tar.gz"
