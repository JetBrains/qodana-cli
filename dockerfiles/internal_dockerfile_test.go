package dockerfiles

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// internal.Dockerfile builds FROM this branch's base images: *-latest on main, *-NNN on a release branch NNN
// (BASE_BRANCH in base/docker-bake.hcl). A wrong BASE_SUFFIX silently builds on another branch's base.

var (
	suffixArgRe   = regexp.MustCompile(`(?m)^ARG BASE_SUFFIX="([^"]*)"$`)
	baseFromRe    = regexp.MustCompile(`(?m)^FROM registry\.jetbrains\.team/p/sa/containers/qodana:(\S+)$`)
	releaseBranch = regexp.MustCompile(`^[0-9]{3}$`)
)

func baseSuffixes(t *testing.T) map[string]string {
	paths, err := filepath.Glob(filepath.Join("*", "internal.Dockerfile"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no */internal.Dockerfile found: %v", err)
	}
	suffixes := map[string]string{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		content := strings.ReplaceAll(string(data), "\r\n", "\n")
		global := content
		if i := strings.Index("\n"+content, "\nFROM "); i >= 0 {
			global = content[:i]
		}
		values := suffixArgRe.FindAllStringSubmatch(global, -1)
		if len(values) != 1 {
			t.Errorf("%s: needs one global ARG BASE_SUFFIX before the first FROM, found %d", path, len(values))
			continue
		}
		bases := baseFromRe.FindAllStringSubmatch(content, -1)
		if len(bases) == 0 {
			t.Errorf("%s: no FROM of a qodana base image", path)
		}
		for _, base := range bases {
			if !strings.HasSuffix(base[1], "-$BASE_SUFFIX") {
				t.Errorf("%s: base image %s must end with -$BASE_SUFFIX", path, base[1])
			}
		}
		suffixes[filepath.Dir(path)] = values[0][1]
	}
	return suffixes
}

func TestInternalDockerfilesUseOneBaseSuffix(t *testing.T) {
	seen := map[string][]string{}
	for linter, suffix := range baseSuffixes(t) {
		seen[suffix] = append(seen[suffix], linter)
	}
	if len(seen) > 1 {
		t.Errorf("BASE_SUFFIX differs between linters: %v", seen)
	}
}

func TestInternalDockerfilesBaseSuffixMatchesBranch(t *testing.T) {
	// The PR's target branch, or the pushed branch; unknown outside GitHub Actions
	branch := os.Getenv("GITHUB_BASE_REF")
	if branch == "" {
		branch = os.Getenv("GITHUB_REF_NAME")
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" || branch == "" {
		t.Skip("branch is known only in GitHub Actions")
	}
	expected := "latest"
	if releaseBranch.MatchString(branch) {
		expected = branch
	}
	for linter, suffix := range baseSuffixes(t) {
		if suffix != expected {
			t.Errorf("%s: BASE_SUFFIX is %q on branch %s, expected %q", linter, suffix, branch, expected)
		}
	}
}
