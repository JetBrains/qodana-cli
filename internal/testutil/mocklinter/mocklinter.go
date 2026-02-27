// Package mocklinter provides mock product.Analyzer implementations backed
// by the mockexe callback framework. Tests get a one-call setup for either
// native or Docker mode.
package mocklinter

import (
	"path/filepath"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/testutil/mockexe"
)

const (
	// MockBuild and MockVersion are used by both Native and Docker setups.
	MockBuild   = "261.0"
	MockVersion = "2026.1"
)

// Linter is a product.Linter representing a mock analysis tool.
var Linter = product.Linter{
	Name:            "qodana-mock",
	PresentableName: "Qodana for Mocking",
	ProductCode:     "QDMOCK",
	DockerImage:     "local/qodana-mock",
	SupportNative:   true,
	IsPaid:          false,
	SupportFixes:    false,
	EapOnly:         false,
}

// Native creates a mock native analyzer exe. The handler runs in the test
// process when production code invokes the IDE script. Returns the exe path.
func Native(t testing.TB, handler func(inv *mockexe.CallContext) int) string {
	t.Helper()

	dir := t.TempDir()
	return mockexe.CreateMockExe(t, filepath.Join(dir, "ide"), handler)
}
