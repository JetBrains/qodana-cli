package core

import (
	"fmt"
	"testing"

	"github.com/JetBrains/qodana-cli/v2025/platform/product"
	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
	"github.com/stretchr/testify/assert"
)

func TestImageChecks(t *testing.T) {
	testCases := []struct {
		linter          string
		isUnofficial    bool
		hasExactVersion bool
		isCompatible    bool
	}{
		{
			"hadolint",
			true,
			false,
			false,
		},
		{
			"jetbrains/qodana",
			false,
			false,
			false,
		},
		{
			"jetbrains/qodana:latest",
			false,
			false,
			false,
		},
		{
			"jetbrains/qodana:2022.1",
			false,
			true,
			false,
		},
		{
			fmt.Sprintf("jetbrains/qodana:%s", product.ReleaseVersion),
			false,
			true,
			true,
		},
	}
	for _, tc := range testCases {
		t.Run(
			tc.linter, func(t *testing.T) {
				if isUnofficialLinter(tc.linter) != tc.isUnofficial {
					t.Errorf("isUnofficial: got %v, want %v", isUnofficialLinter(tc.linter), tc.isUnofficial)
				}
				if hasExactVersionTag(tc.linter) != tc.hasExactVersion {
					t.Errorf("hasExactVersion: got %v, want %v", hasExactVersionTag(tc.linter), tc.hasExactVersion)
				}
				if isCompatibleLinter(tc.linter) != tc.isCompatible {
					t.Errorf("isCompatible: got %v, want %v", isCompatibleLinter(tc.linter), tc.isCompatible)
				}
			},
		)
	}
}

func TestSelectUser(t *testing.T) {
	// auto implies selecting a user automatically for non-priveleged images
	assert.Equal(t, selectUser("jetbrains/qodana-cpp:2025.2-eap-clang18", "auto"), utils.GetDefaultUser())
	assert.Equal(t, selectUser("jetbrains/qodana-cpp:2025.2-eap-clang18-privileged", "auto"), "")

	// Explicitly specified UIDs should not be overridden
	assert.Equal(t, selectUser("jetbrains/qodana-cpp:2025.2-eap-clang18", "0"), "0")
	assert.Equal(t, selectUser("jetbrains/qodana-cpp:2025.2-eap-clang18-privileged", "0"), "0")
	assert.Equal(t, selectUser("jetbrains/qodana-cpp:2025.2-eap-clang18", "1337"), "1337")
	assert.Equal(t, selectUser("jetbrains/qodana-cpp:2025.2-eap-clang18-privileged", "1337"), "1337")

	// Internal registry is supported
	assert.Equal(t, selectUser("registry.jetbrains.team/qodana-cpp:2025.2-eap-clang18", "auto"), utils.GetDefaultUser())
	assert.Equal(t, selectUser("registry.jetbrains.team/qodana-cpp:2025.2-eap-clang18-privileged", "auto"), "")

	// User-specified images are unaffected
	assert.Equal(t, selectUser("myregistry.local/qodana-cpp:2025.2-eap-clang18", "auto"), utils.GetDefaultUser())
	assert.Equal(t, selectUser("myregistry.local/qodana-cpp:2025.2-eap-clang18-privileged", "auto"), utils.GetDefaultUser())
}
