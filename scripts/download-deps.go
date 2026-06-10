//go:build ignore

// Thin shim invoked from `//go:generate` in clang/run.go and cdnet/run.go. It downloads (and
// sha256-verifies against the committed pin) the closed-source linter archives for one dependency
// from JB Space. All logic lives in the testable scripts/downloaddeps package. See QD-14839.
//
// Usage:
//
//	go run scripts/download-deps.go <clang-tidy|cdnet>
//
// Env: QODANA_CLI_DEPS_TOKEN (Space read token; absent => empty placeholders),
// QODANA_CLI_DEPS_FORCE=1 (re-download and rewrite the pin's hashes),
// QODANA_CLI_DEPS_ALL=1 (fetch every platform, not just the runner's).
package main

import (
	"log"
	"os"

	"github.com/JetBrains/qodana-cli/scripts/downloaddeps"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("usage: go run scripts/download-deps.go <clang-tidy|cdnet>")
	}
	if err := downloaddeps.Main(os.Args[1]); err != nil {
		log.Fatal(err)
	}
}
