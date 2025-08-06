package main

import _ "embed"

//go:embed clang-tidy-linux-amd64.tar.gz
var ClangTidyArchive []byte

//go:embed clang-tidy-linux-amd64.tar.gz.sha256.bin
var ClangTidySha256 []byte
