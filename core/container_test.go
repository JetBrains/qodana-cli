package core

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2025/platform/product"
	"github.com/JetBrains/qodana-cli/v2025/platform/qdenv"
	"github.com/docker/docker/api/types/backend"
	"github.com/docker/docker/api/types/container"
	"slices"
	"strings"
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

func TestWithoutEnv(t *testing.T) {
	env := []string{"QODANA_ORG_TOKEN=org", "QODANA_ORG_TOKEN_X=keep", "QODANA_TOKEN=token", "OTHER=QODANA_ORG_TOKEN=x"}
	expected := []string{"QODANA_ORG_TOKEN_X=keep", "QODANA_TOKEN=token", "OTHER=QODANA_ORG_TOKEN=x"}
	if actual := withoutEnv(env, qdenv.QodanaOrgToken); !slices.Equal(actual, expected) {
		t.Errorf("expected %v, got %v", expected, actual)
	}
}

func TestDebugDockerRunCommandHidesTokens(t *testing.T) {
	cfg := &backend.ContainerCreateConfig{
		Config: &container.Config{
			Image: "image",
			Env:   []string{"QODANA_ORG_TOKEN=org-secret", "QODANA_TOKEN=project-secret", "QODANA_BRANCH=main"},
		},
	}
	command := generateDebugDockerRunCommand(cfg)
	for _, secret := range []string{"org-secret", "project-secret"} {
		if strings.Contains(command, secret) {
			t.Errorf("debug command must not contain '%s': %s", secret, command)
		}
	}
	if !strings.Contains(command, "-e QODANA_BRANCH=main") {
		t.Errorf("debug command must contain non-secret env: %s", command)
	}
}
