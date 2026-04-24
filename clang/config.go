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

const clangTidySearchRootEnv = "QODANA_CLANG_TIDY_SEARCH_ROOT"

// resolvePath returns EvalSymlinks(p) when it succeeds, otherwise Abs(p).
// EvalSymlinks fails when the path doesn't exist; Abs is good enough there.
func resolvePath(p string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved, nil
	}
	return filepath.Abs(p)
}

// findClangTidyConfig returns the absolute path of the nearest .clang-tidy
// or _clang-tidy file reachable from startDir by walking toward the
// filesystem root. Returns "" if none is found.
//
// Matches clang-tidy's own discovery (FileOptionsBaseProvider::addRawFileOptions):
// walk to the filesystem root, no repo/$HOME boundary. This keeps our
// scanner profile consistent with whatever clang-tidy itself would pick up.
//
// The QODANA_CLANG_TIDY_SEARCH_ROOT env var caps the walk at a specific
// directory (inclusive: the searchRoot is still checked; its parents are
// not). If set, startDir must be equal to or a descendant of searchRoot —
// otherwise this function returns an error. The var is intended for tests;
// production never sets it. A Warn is logged when it is set so accidental
// production use is observable.
//
// Within a single directory, .clang-tidy is preferred over _clang-tidy
// (iteration order of clangTidyConfigNames). Upstream clang-tidy only
// recognizes .clang-tidy; _clang-tidy is a Qodana-specific convenience.
func findClangTidyConfig(startDir string) (string, error) {
	dir, err := resolvePath(startDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path for %s: %w", startDir, err)
	}

	var searchRoot string
	if raw := os.Getenv(clangTidySearchRootEnv); raw != "" {
		searchRoot, err = resolvePath(raw)
		if err != nil {
			return "", fmt.Errorf("failed to resolve %s=%q: %w", clangTidySearchRootEnv, raw, err)
		}
		log.Warnf("%s is set — this is intended for tests only and restricts .clang-tidy discovery to %s",
			clangTidySearchRootEnv, searchRoot)

		rel, relErr := filepath.Rel(searchRoot, dir)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("%s=%q is not an ancestor of start dir %q",
				clangTidySearchRootEnv, searchRoot, dir)
		}
	}

	for {
		for _, name := range clangTidyConfigNames {
			candidate := filepath.Join(dir, name)
			_, err := os.Stat(candidate)
			if err == nil {
				log.Debugf("Found %s in %s", name, dir)
				return candidate, nil
			}
			if !errors.Is(err, os.ErrNotExist) {
				log.Debugf("Error checking for %s in %s: %v", name, dir, err)
			}
		}
		if searchRoot != "" && dir == searchRoot {
			return "", nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

// processConfig reads qodana.yaml includes/excludes, detects .clang-tidy /
// _clang-tidy config files in parent directories, and builds the --checks=
// argument for clang-tidy. When .clang-tidy exists it is the base; otherwise
// a curated default set is used.
//
// The returned configFile is non-empty only when the found config file is
// named _clang-tidy — clang-tidy's native per-file walk recognizes only
// .clang-tidy, so _clang-tidy must be passed explicitly via --config-file=.
// For .clang-tidy we leave configFile empty so clang-tidy's native walk
// continues to work (this preserves per-directory .clang-tidy discovery for
// source files in subdirectories). findClangTidyConfig returns an
// EvalSymlinks-resolved path, so the configFile here is already resolved.
func processConfig(c thirdpartyscan.Context) (checks string, configFile string, err error) {
	var excludeRules []string
	var includeRules []string

	yaml := c.QodanaYamlConfig()
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

	configPath, err := findClangTidyConfig(c.ProjectDir())
	if err != nil {
		return "", "", err
	}
	hasConfig := configPath != ""
	// clang-tidy's native walk hard-codes ".clang-tidy". Any other filename
	// (currently just "_clang-tidy") must be forwarded via --config-file=
	// because clang-tidy's own discovery will not find it.
	if hasConfig && filepath.Base(configPath) != ".clang-tidy" {
		configFile = configPath
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
	return checks, configFile, nil
}
