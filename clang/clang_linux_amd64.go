//go:build linux && amd64

package main

import _ "embed"

//go:embed clang-linux-x64.tar.gz
var Clang []byte
var Ext = ".tar.gz"
