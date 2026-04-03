package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/internal/platform/utils"
	log "github.com/sirupsen/logrus"
)

// defaultChecks is a curated set of clang-tidy checks enabled when no .clang-tidy
// config file exists. Covers universally useful categories while excluding
// platform/codebase-specific ones (llvm-*, fuchsia-*, google-*, etc.) and
// individual checks that are too noisy without per-project tuning.
var defaultChecks = strings.Join([]string{
	"-*", // Disable everything first, then enable curated categories.
	"bugprone-*",
	"cert-*",
	"clang-analyzer-*",
	"clang-diagnostic-*",
	"concurrency-*",
	"misc-*",
	"modernize-*",
	"performance-*",
	"portability-*",
	"readability-*",
	// Disabled individual checks (noisy without per-project tuning)
	"-misc-confusable-identifiers",
	"-misc-include-cleaner",
	"-misc-no-recursion",
	"-misc-non-private-member-variables-in-classes",
	"-modernize-use-trailing-return-type",
	"-readability-identifier-length",
	"-readability-magic-numbers",
}, ",")

var clangTidyConfigNames = []string{".clang-tidy", "_clang-tidy"}

// findClangTidyConfig walks from startDir toward the filesystem root,
// looking for a .clang-tidy or _clang-tidy file. Returns true if found.
// Approximates clang-tidy's parent-directory config file lookup.
func findClangTidyConfig(startDir string) (bool, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return false, fmt.Errorf("failed to resolve absolute path for %s: %w", startDir, err)
	}
	for {
		for _, name := range clangTidyConfigNames {
			_, err := os.Stat(filepath.Join(dir, name))
			if err == nil {
				log.Debugf("Found %s in %s", name, dir)
				return true, nil
			}
			if !errors.Is(err, os.ErrNotExist) {
				// Permission error, broken symlink, etc. — skip, don't abort.
				log.Debugf("Error checking for %s in %s: %v", name, dir, err)
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir { // filesystem root
			return false, nil
		}
		dir = parent
	}
}

// processConfig reads qodana.yaml includes/excludes, detects .clang-tidy config
// files in parent directories, and builds the --checks= argument for clang-tidy.
// When .clang-tidy exists it is the base; otherwise a curated default set is used.
func processConfig(c thirdpartyscan.Context) (string, error) {
	var excludeRules []string
	var includeRules []string

	yaml := c.QodanaYamlConfig()
	var checks string
	utils.Bootstrap(yaml.Bootstrap, c.ProjectDir())
	if yaml.Version != "" || len(yaml.Includes) > 0 || len(yaml.Excludes) > 0 {
		fmt.Println("Found qodana.yaml. Note that only bootstrap command and inspection names from include and exclude sections are supported.")
		for _, include := range yaml.Includes {
			if strings.HasPrefix(strings.TrimSpace(include.Name), "clion-") {
				continue
			}
			if strings.ContainsAny(include.Name, "\"") {
				log.Warnf("Skipping include rule with invalid characters: %s", include.Name)
				continue
			}
			includeRules = append(includeRules, include.Name)
		}
		for _, exclude := range yaml.Excludes {
			if strings.ContainsAny(exclude.Name, "\"") {
				log.Warnf("Skipping exclude rule with invalid characters: %s", exclude.Name)
				continue
			}
			excludeRules = append(excludeRules, exclude.Name)
		}
	}
	for i, rule := range excludeRules {
		excludeRules[i] = "-" + rule
	}

	hasConfig, err := findClangTidyConfig(c.ProjectDir())
	if err != nil {
		return "", err
	}

	// When .clang-tidy exists: it is the base, includes/excludes layer on top.
	// When no .clang-tidy: defaults are the base, includes/excludes layer on top.
	parts := make([]string, 0, len(includeRules)+len(excludeRules))
	parts = append(parts, includeRules...)
	parts = append(parts, excludeRules...)
	overrides := strings.Join(parts, ",")

	switch {
	case hasConfig && overrides != "":
		checks = fmt.Sprintf("--checks=%s", overrides)
	case hasConfig:
		// Let clang-tidy use its own config unmodified.
	case overrides != "":
		checks = fmt.Sprintf("--checks=%s,%s", defaultChecks, overrides)
	default:
		checks = fmt.Sprintf("--checks=%s", defaultChecks)
	}
	return checks, nil
}
