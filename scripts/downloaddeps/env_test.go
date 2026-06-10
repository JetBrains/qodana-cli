package downloaddeps

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnv_ReadsKeyValues(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("# comment\nexport FOO=bar\nBAZ=qux\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := loadEnv(envFile)
	if got["FOO"] != "bar" {
		t.Errorf("FOO = %q, want bar", got["FOO"])
	}
	if got["BAZ"] != "qux" {
		t.Errorf("BAZ = %q, want qux", got["BAZ"])
	}
}

func TestLoadEnv_DoesNotMutateGlobalEnv(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("QD_DOWNLOADDEPS_PROBE=fromfile\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = loadEnv(envFile)
	if v, ok := os.LookupEnv("QD_DOWNLOADDEPS_PROBE"); ok {
		t.Errorf("loadEnv mutated global env: QD_DOWNLOADDEPS_PROBE=%q", v)
	}
}

func TestLoadEnv_MissingFileReturnsEmpty(t *testing.T) {
	got := loadEnv(filepath.Join(t.TempDir(), "nope.env"))
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestResolveEnv_EnvWinsOverFile(t *testing.T) {
	t.Setenv("QD_DOWNLOADDEPS_WIN", "fromenv")
	file := map[string]string{"QD_DOWNLOADDEPS_WIN": "fromfile"}
	if got := resolveEnv("QD_DOWNLOADDEPS_WIN", file); got != "fromenv" {
		t.Errorf("resolveEnv = %q, want fromenv", got)
	}
}

func TestResolveEnv_FallsBackToFile(t *testing.T) {
	file := map[string]string{"QD_DOWNLOADDEPS_ONLYFILE": "fromfile"}
	if got := resolveEnv("QD_DOWNLOADDEPS_ONLYFILE", file); got != "fromfile" {
		t.Errorf("resolveEnv = %q, want fromfile", got)
	}
}

func TestResolveEnv_ExplicitEmptyOverridesFile(t *testing.T) {
	t.Setenv("QD_DOWNLOADDEPS_EMPTY", "")
	file := map[string]string{"QD_DOWNLOADDEPS_EMPTY": "fromfile"}
	if got := resolveEnv("QD_DOWNLOADDEPS_EMPTY", file); got != "" {
		t.Errorf("resolveEnv = %q, want empty (explicit empty env must override .env)", got)
	}
}
