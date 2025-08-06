package main

import _ "embed"

//go:embed clang-tidy-windows-arm64.zip
var ClangTidyArchive []byte

//go:embed clang-tidy-windows-arm64.zip.sha256.bin
var ClangTidySha256 []byte
