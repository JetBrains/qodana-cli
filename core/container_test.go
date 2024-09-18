package core

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"testing"
)

func TestImageChecks(t *testing.T) {
	testCases := []struct {
		linter          string
		newLinter       string
		isUnofficial    bool
		hasExactVersion bool
		isCompatible    bool
	}{
		{
			"hadolint",
			"hadolint",
			true,
			false,
			false,
		},
		{
			"jetbrains/qodana",
			fmt.Sprintf("jetbrains/qodana:%s", platform.ReleaseVersion),
			false,
			false,
			false,
		},
		{
			"jetbrains/qodana:latest",
			fmt.Sprintf("jetbrains/qodana:%s", platform.ReleaseVersion),
			false,
			false,
			false,
		},
		{
			"jetbrains/qodana:2022.1",
			"jetbrains/qodana:2022.1",
			false,
			true,
			false,
		},
		{
			fmt.Sprintf("jetbrains/qodana:%s", platform.ReleaseVersion),
			fmt.Sprintf("jetbrains/qodana:%s", platform.ReleaseVersion),
			false,
			true,
			true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.linter, func(t *testing.T) {
			platform.Version = platform.ReleaseVersion + ".0"
			newLinter := checkImage(tc.linter)
			if newLinter != tc.newLinter {
				t.Errorf("checkImage: got %v, want %v", newLinter, tc.newLinter)
			}
			if isUnofficialLinter(tc.linter) != tc.isUnofficial {
				t.Errorf("isUnofficial: got %v, want %v", isUnofficialLinter(tc.linter), tc.isUnofficial)
			}
			if hasExactVersionTag(tc.linter) != tc.hasExactVersion {
				t.Errorf("hasExactVersion: got %v, want %v", hasExactVersionTag(tc.linter), tc.hasExactVersion)
			}
			if isCompatibleLinter(tc.linter) != tc.isCompatible {
				t.Errorf("isCompatible: got %v, want %v", isCompatibleLinter(tc.linter), tc.isCompatible)
			}
		})
	}
}
