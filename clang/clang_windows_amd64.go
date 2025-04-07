//go:build windows && amd64

package main

import _ "embed"

//go:embed clang-win-x64.zip
var Clang []byte
var Ext = ".zip"
