package downloaddeps

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRepo lays out a temp repo root with the given pin config at
// scripts/downloaddeps/<name>.json and an empty dest dir, then returns the root.
func fakeRepo(t *testing.T, name string, cfg Config) string {
	t.Helper()
	root := t.TempDir()
	ddir := filepath.Join(root, "scripts", "downloaddeps")
	if err := os.MkdirAll(ddir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ddir, name+".json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, cfg.DestDir), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// muxServer serves a body per filename (matched as the last URL path segment) and counts hits.
func muxServer(bodies map[string][]byte, hits *int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			*hits++
		}
		name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		if body, ok := bodies[name]; ok {
			_, _ = w.Write(body)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
}

func readCfg(t *testing.T, root, name string) Config {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "scripts", "downloaddeps", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := parseConfig(b)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestRun_NoToken_WritesPlaceholders(t *testing.T) {
	cfg := Config{Version: "v1", URL: "http://unused/$version/$filename", DestDir: "cdnet",
		Sha256: map[string]string{"clt.zip": "abc"}}
	root := fakeRepo(t, "cdnet", cfg)

	hits := 0
	srv := muxServer(nil, &hits)
	defer srv.Close()

	if err := run(srv.Client(), root, "cdnet", "linux", "amd64", "", false, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	if hits != 0 {
		t.Errorf("expected no download in placeholder mode, got %d hits", hits)
	}
	dest := filepath.Join(root, "cdnet", "clt.zip")
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("placeholder not created: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("placeholder size = %d, want 0", info.Size())
	}
}

func TestRun_NoToken_PreservesExistingFile(t *testing.T) {
	cfg := Config{Version: "v1", URL: "http://unused/$version/$filename", DestDir: "cdnet",
		Sha256: map[string]string{"clt.zip": "abc"}}
	root := fakeRepo(t, "cdnet", cfg)
	dest := filepath.Join(root, "cdnet", "clt.zip")
	if err := os.WriteFile(dest, []byte("real archive bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := run(http.DefaultClient, root, "cdnet", "linux", "amd64", "", false, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "real archive bytes" {
		t.Errorf("existing file clobbered: got %q", got)
	}
}

func TestRun_NormalMode_DownloadsAndVerifies(t *testing.T) {
	body := []byte("clt archive")
	sum := hex.EncodeToString(sha256Sum(body))
	hits := 0
	srv := muxServer(map[string][]byte{"clt.zip": body}, &hits)
	defer srv.Close()

	cfg := Config{Version: "v1", URL: srv.URL + "/$version/$filename", DestDir: "cdnet",
		Sha256: map[string]string{"clt.zip": sum}}
	root := fakeRepo(t, "cdnet", cfg)

	if err := run(srv.Client(), root, "cdnet", "linux", "amd64", "tok", false, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(root, "cdnet", "clt.zip"))
	if string(got) != string(body) {
		t.Errorf("downloaded content = %q, want %q", got, body)
	}
	if hits != 1 {
		t.Errorf("expected 1 download, got %d", hits)
	}
	// Normal mode must not rewrite the pin.
	if after := readCfg(t, root, "cdnet"); after.Sha256["clt.zip"] != sum {
		t.Errorf("pin mutated in normal mode: %q", after.Sha256["clt.zip"])
	}
}

func TestRun_CacheHit_NoDownload(t *testing.T) {
	body := []byte("cached clt")
	sum := hex.EncodeToString(sha256Sum(body))
	hits := 0
	srv := muxServer(map[string][]byte{"clt.zip": body}, &hits)
	defer srv.Close()

	cfg := Config{Version: "v1", URL: srv.URL + "/$version/$filename", DestDir: "cdnet",
		Sha256: map[string]string{"clt.zip": sum}}
	root := fakeRepo(t, "cdnet", cfg)
	if err := os.WriteFile(filepath.Join(root, "cdnet", "clt.zip"), body, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := run(srv.Client(), root, "cdnet", "linux", "amd64", "tok", false, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	if hits != 0 {
		t.Errorf("expected cache hit (0 downloads), got %d", hits)
	}
}

// clangFiles is the full six-platform key set used by the clang-tidy pin.
var clangFiles = []string{
	"clang-tidy-linux-amd64.tar.gz", "clang-tidy-linux-arm64.tar.gz",
	"clang-tidy-darwin-amd64.tar.gz", "clang-tidy-darwin-arm64.tar.gz",
	"clang-tidy-windows-amd64.zip", "clang-tidy-windows-arm64.zip",
}

func TestRun_NoToken_MultiPlatform_OnlyHostPlaceholder(t *testing.T) {
	sha := map[string]string{}
	for _, f := range clangFiles {
		sha[f] = ""
	}
	cfg := Config{Version: "v1", URL: "http://unused/$version/$filename", DestDir: "clang", Sha256: sha}
	root := fakeRepo(t, "clang-tidy", cfg)

	if err := run(http.DefaultClient, root, "clang-tidy", "linux", "amd64", "", false, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, f := range clangFiles {
		_, err := os.Stat(filepath.Join(root, "clang", f))
		isHost := f == "clang-tidy-linux-amd64.tar.gz"
		if isHost && err != nil {
			t.Errorf("expected host placeholder %s, missing: %v", f, err)
		}
		if !isHost && err == nil {
			t.Errorf("non-host file %s must not be created in placeholder mode", f)
		}
	}
}

func TestRun_NormalMode_MultiPlatform_OnlySelected(t *testing.T) {
	amd := "clang-tidy-linux-amd64.tar.gz"
	arm := "clang-tidy-linux-arm64.tar.gz"
	win := "clang-tidy-windows-amd64.zip"
	bodies := map[string][]byte{amd: []byte("AMD"), arm: []byte("ARM"), win: []byte("WIN")}
	sha := map[string]string{}
	for f, b := range bodies {
		sha[f] = hex.EncodeToString(sha256Sum(b))
	}
	hits := 0
	srv := muxServer(bodies, &hits)
	defer srv.Close()
	cfg := Config{Version: "v1", URL: srv.URL + "/$version/$filename", DestDir: "clang", Sha256: sha}
	root := fakeRepo(t, "clang-tidy", cfg)

	if err := run(srv.Client(), root, "clang-tidy", "linux", "amd64", "tok", false, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	if hits != 1 {
		t.Errorf("expected exactly 1 download (host only), got %d", hits)
	}
	if _, err := os.Stat(filepath.Join(root, "clang", amd)); err != nil {
		t.Errorf("host file not downloaded: %v", err)
	}
	for _, f := range []string{arm, win} {
		if _, err := os.Stat(filepath.Join(root, "clang", f)); err == nil {
			t.Errorf("non-host file %s must not be downloaded", f)
		}
	}
}

func TestRun_MissingConfig_Errors(t *testing.T) {
	if err := run(http.DefaultClient, t.TempDir(), "nope", "linux", "amd64", "tok", false, false); err == nil {
		t.Fatal("expected error for missing config, got nil")
	}
}

func TestRun_MalformedConfig_Errors(t *testing.T) {
	root := t.TempDir()
	ddir := filepath.Join(root, "scripts", "downloaddeps")
	if err := os.MkdirAll(ddir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ddir, "bad.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(http.DefaultClient, root, "bad", "linux", "amd64", "tok", false, false); err == nil {
		t.Fatal("expected error for malformed config, got nil")
	}
}

func TestRun_RejectsBadFilenameKey(t *testing.T) {
	cfg := Config{Version: "v1", URL: "http://unused/$filename", DestDir: "cdnet",
		Sha256: map[string]string{"../escape": ""}}
	root := fakeRepo(t, "cdnet", cfg)
	if err := run(http.DefaultClient, root, "cdnet", "linux", "amd64", "", false, false); err == nil {
		t.Fatal("expected error for path-traversal filename key, got nil")
	}
}

// TestMain_ForceWithoutTokenFromDotEnv exercises Main's wiring end-to-end: it must locate the repo
// root via go.mod, read QODANA_CLI_DEPS_FORCE from .env, and fail loud (force without a token).
func TestMain_ForceWithoutTokenFromDotEnv(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ddir := filepath.Join(root, "scripts", "downloaddeps")
	if err := os.MkdirAll(ddir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Version: "v1", URL: "http://unused/$filename", DestDir: "cdnet",
		Sha256: map[string]string{"clt.zip": ""}}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(filepath.Join(ddir, "cdnet.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("QODANA_CLI_DEPS_FORCE=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QODANA_CLI_DEPS_TOKEN", "") // explicit empty: no token regardless of ambient env
	t.Setenv("QODANA_CLI_DEPS_ALL", "")
	t.Chdir(root)

	if err := Main("cdnet"); err == nil {
		t.Fatal("expected force-without-token error driven by .env FORCE, got nil")
	}
}

func TestRun_Mismatch_Errors(t *testing.T) {
	body := []byte("actual")
	srv := muxServer(map[string][]byte{"clt.zip": body}, nil)
	defer srv.Close()
	cfg := Config{Version: "v1", URL: srv.URL + "/$version/$filename", DestDir: "cdnet",
		Sha256: map[string]string{"clt.zip": "deadbeef"}}
	root := fakeRepo(t, "cdnet", cfg)

	if err := run(srv.Client(), root, "cdnet", "linux", "amd64", "tok", false, false); err == nil {
		t.Fatal("expected sha mismatch error, got nil")
	}
	if _, err := os.Stat(filepath.Join(root, "cdnet", "clt.zip")); err == nil {
		t.Error("mismatched download should not be left on disk")
	}
}

func TestRun_Force_MergesOnlySelectedHash(t *testing.T) {
	amd64 := "clang-tidy-linux-amd64.tar.gz"
	arm64 := "clang-tidy-linux-arm64.tar.gz"
	body := []byte("real amd64 archive")
	want := hex.EncodeToString(sha256Sum(body))

	srv := muxServer(map[string][]byte{amd64: body}, nil)
	defer srv.Close()
	cfg := Config{Version: "v1", URL: srv.URL + "/$version/$filename", DestDir: "clang",
		Sha256: map[string]string{amd64: "stale-amd64", arm64: "untouched-arm64"}}
	root := fakeRepo(t, "clang-tidy", cfg)

	// force, per-platform (all=false) on linux/amd64: only amd64 is selected.
	if err := run(srv.Client(), root, "clang-tidy", "linux", "amd64", "tok", true, false); err != nil {
		t.Fatalf("run force: %v", err)
	}
	after := readCfg(t, root, "clang-tidy")
	if after.Sha256[amd64] != want {
		t.Errorf("amd64 hash = %q, want refreshed %q", after.Sha256[amd64], want)
	}
	if after.Sha256[arm64] != "untouched-arm64" {
		t.Errorf("arm64 hash = %q, want preserved untouched-arm64", after.Sha256[arm64])
	}
}

func TestRun_ForceNoToken_Errors(t *testing.T) {
	cfg := Config{Version: "v1", URL: "http://unused/$version/$filename", DestDir: "cdnet",
		Sha256: map[string]string{"clt.zip": ""}}
	root := fakeRepo(t, "cdnet", cfg)
	if err := run(http.DefaultClient, root, "cdnet", "linux", "amd64", "", true, false); err == nil {
		t.Fatal("expected error for force without token, got nil")
	}
}

func TestRun_All_DownloadsEveryFile(t *testing.T) {
	a := "clang-tidy-linux-amd64.tar.gz"
	b := "clang-tidy-darwin-arm64.tar.gz"
	ba, bb := []byte("aaa"), []byte("bbb")
	cfg := Config{Version: "v1", DestDir: "clang", Sha256: map[string]string{
		a: hex.EncodeToString(sha256Sum(ba)),
		b: hex.EncodeToString(sha256Sum(bb)),
	}}
	srv := muxServer(map[string][]byte{a: ba, b: bb}, nil)
	defer srv.Close()
	cfg.URL = srv.URL + "/$version/$filename"
	root := fakeRepo(t, "clang-tidy", cfg)

	// host is linux/amd64 but all=true must fetch the darwin file too.
	if err := run(srv.Client(), root, "clang-tidy", "linux", "amd64", "tok", false, true); err != nil {
		t.Fatalf("run all: %v", err)
	}
	for _, f := range []string{a, b} {
		if _, err := os.Stat(filepath.Join(root, "clang", f)); err != nil {
			t.Errorf("expected %s downloaded with all=true: %v", f, err)
		}
	}
}

func TestRun_RejectsEscapingDestDir(t *testing.T) {
	cfg := Config{Version: "v1", URL: "http://unused/$f", DestDir: "../escape",
		Sha256: map[string]string{"clt.zip": ""}}
	root := fakeRepo(t, "cdnet", cfg)
	if err := run(http.DefaultClient, root, "cdnet", "linux", "amd64", "", false, false); err == nil {
		t.Fatal("expected error for dest_dir escaping the repo root, got nil")
	}
}

func TestResolveTarget(t *testing.T) {
	t.Run("honors TARGETOS/TARGETARCH", func(t *testing.T) {
		t.Setenv("TARGETOS", "windows")
		t.Setenv("TARGETARCH", "arm64")
		goos, goarch := resolveTarget()
		if goos != "windows" || goarch != "arm64" {
			t.Errorf("resolveTarget = (%q,%q), want (windows,arm64)", goos, goarch)
		}
	})
	t.Run("falls back to runtime", func(t *testing.T) {
		t.Setenv("TARGETOS", "")
		t.Setenv("TARGETARCH", "")
		goos, goarch := resolveTarget()
		if goos == "" || goarch == "" {
			t.Errorf("resolveTarget runtime fallback empty: (%q,%q)", goos, goarch)
		}
	})
}

func TestFindRepoRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := findRepoRoot(nested)
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	if got != root {
		t.Errorf("findRepoRoot = %q, want %q", got, root)
	}

	t.Run("errors when no go.mod up the tree", func(t *testing.T) {
		bare := t.TempDir() // no go.mod
		if _, err := findRepoRoot(bare); err == nil {
			t.Error("expected error when no go.mod found, got nil")
		}
	})
}

func TestMain_RejectsBadConfigName(t *testing.T) {
	for _, name := range []string{"../evil", "a/b", "a\\b", "", ".", ".."} {
		if err := Main(name); err == nil {
			t.Errorf("Main(%q) = nil, want error", name)
		}
	}
}

func TestTruthy(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "True"} {
		if !truthy(v) {
			t.Errorf("truthy(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"", "0", "yes", "no", "false"} {
		if truthy(v) {
			t.Errorf("truthy(%q) = true, want false", v)
		}
	}
}
