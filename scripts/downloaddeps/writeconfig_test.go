package downloaddeps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteConfig_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clang-tidy.json")
	cfg := Config{
		Version: "v1.0.0",
		URL:     "https://host/clang-tidy/$version/$filename",
		DestDir: "clang",
		Sha256: map[string]string{
			"clang-tidy-linux-amd64.tar.gz":  "aaa",
			"clang-tidy-darwin-arm64.tar.gz": "bbb",
		},
	}
	if err := writeConfig(path, cfg); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseConfig(b)
	if err != nil {
		t.Fatalf("parseConfig of written file: %v", err)
	}
	if got.Version != cfg.Version || got.URL != cfg.URL || got.DestDir != cfg.DestDir {
		t.Errorf("round-trip scalar mismatch: %+v", got)
	}
	for k, v := range cfg.Sha256 {
		if got.Sha256[k] != v {
			t.Errorf("round-trip sha256[%q] = %q, want %q", k, got.Sha256[k], v)
		}
	}
}

func TestWriteConfig_DeterministicAndTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.json")
	p2 := filepath.Join(dir, "b.json")
	cfg := Config{Version: "v1", URL: "u", DestDir: "clang", Sha256: map[string]string{
		"z": "1", "a": "2", "m": "3",
	}}
	if err := writeConfig(p1, cfg); err != nil {
		t.Fatal(err)
	}
	if err := writeConfig(p2, cfg); err != nil {
		t.Fatal(err)
	}
	b1, _ := os.ReadFile(p1)
	b2, _ := os.ReadFile(p2)
	if string(b1) != string(b2) {
		t.Errorf("writeConfig not deterministic:\n%s\n---\n%s", b1, b2)
	}
	if !strings.HasSuffix(string(b1), "\n") {
		t.Error("writeConfig output missing trailing newline")
	}
	// keys must be sorted (a, m, z)
	s := string(b1)
	ia, im, iz := strings.Index(s, `"a"`), strings.Index(s, `"m"`), strings.Index(s, `"z"`)
	if ia >= im || im >= iz {
		t.Errorf("sha256 keys not sorted: a@%d m@%d z@%d", ia, im, iz)
	}
}
