package dockertest

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
)

// ReexecInDocker re-executes the current test inside a Docker container
// defined by a compose file. The compose file path is relative to the
// caller's source file directory.
//
// Returns true when called inside the container (test should proceed
// with its actual logic). Returns false after the containerized test
// completes successfully (the outer test should return immediately).
// Calls t.Fatal on any failure.
//
// The helper:
//  1. Detects QT_REEXEC_IN_DOCKER=1 → returns true
//  2. Compiles the test binary on the host via `go test -c`
//  3. Runs `docker compose build` (installs packages via Dockerfile — cached)
//  4. Runs `docker compose run --rm -e QT_REEXEC_IN_DOCKER=1` with the
//     pre-compiled binary mounted at /test.bin
//  5. Propagates output and exit code
//
// The compose file should define a "test" service. The helper injects:
//   - QT_REEXEC_IN_DOCKER=1 (via -e flag)
//   - QT_REPO_ROOT (set in the environment for ${QT_REPO_ROOT} interpolation)
//   - The test binary mounted at /test.bin (via -v flag)
//
// The container does NOT need Go or module dependencies — only the
// runtime packages required by the test (e.g., dosfstools).
func ReexecInDocker(t *testing.T, composeFile string) bool {
	t.Helper()

	if os.Getenv("QT_REEXEC_IN_DOCKER") == "1" {
		return true
	}

	root := projectRoot(t)
	pkgPath := callerPackagePath(t, root)
	composeAbs := resolveComposePath(t, composeFile)

	env := append(os.Environ(), "QT_REPO_ROOT="+root)

	// Compile the test binary on the host.
	bin := filepath.Join(t.TempDir(), "test.bin")
	compile := exec.Command("go", "test", "-c", "-o", bin, pkgPath)
	compile.Dir = root
	compile.Env = append(env, "GOOS=linux", "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0")
	out, err := compile.CombinedOutput()
	if err != nil {
		t.Fatalf("go test -c failed: %v\n%s", err, out)
	}

	// Build the service image (installs packages — needs network).
	build := exec.Command("docker", "compose", "-f", composeAbs, "build")
	build.Dir = root
	build.Env = env
	out, err = build.CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose build failed: %v\n%s", err, out)
	}

	// Run the pre-compiled test binary inside the container.
	run := exec.Command("docker", "compose",
		"-f", composeAbs,
		"run", "--rm",
		"-e", "QT_REEXEC_IN_DOCKER=1",
		"-v", bin+":/test.bin:ro",
		"test",
		"/test.bin",
		"-test.run", "^"+regexp.QuoteMeta(t.Name())+"$",
		"-test.v",
	)
	run.Dir = root
	run.Env = env
	out, err = run.CombinedOutput()
	t.Logf("docker compose run output:\n%s", out)
	if err != nil {
		t.Fatalf("test inside container failed: %v", err)
	}

	return false
}

// resolveComposePath resolves the compose file path relative to the caller's
// source file directory (the test file that called ReexecInDocker).
func resolveComposePath(t *testing.T, composeFile string) string {
	t.Helper()
	_, callerFile, _, ok := runtime.Caller(2)
	if !ok {
		t.Fatal("dockertest: could not determine caller file")
	}
	return filepath.Join(filepath.Dir(callerFile), composeFile)
}

// callerPackagePath returns the Go package path (e.g., "./internal/foundation/fs")
// of the test that called ReexecInDocker, relative to the module root.
func callerPackagePath(t *testing.T, root string) string {
	t.Helper()
	_, callerFile, _, ok := runtime.Caller(2)
	if !ok {
		t.Fatal("dockertest: could not determine caller file")
	}
	dir := filepath.Dir(callerFile)
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		t.Fatalf("dockertest: could not compute relative path: %v", err)
	}
	return "./" + filepath.ToSlash(rel)
}

// projectRoot walks up from the current working directory to find go.mod.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("dockertest: Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("dockertest: could not find project root (go.mod)")
		}
		dir = parent
	}
}
