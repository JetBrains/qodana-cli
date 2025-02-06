package core

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform/product"
	"testing"
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
