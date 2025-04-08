//go:build darwin && amd64

package main

import _ "embed"

//go:embed clang-mac-x64.tar.gz
var Clang []byte
var Ext = ".tar.gz"
