package downloaddeps

import "testing"

func TestParseConfig(t *testing.T) {
	data := []byte(`{
	  "version": "v3.0.0",
	  "url": "https://packages.jetbrains.team/files/p/sa/qodana-cli-deps/clang-tidy/$version/$filename",
	  "dest_dir": "clang",
	  "sha256": {
	    "clang-tidy-linux-amd64.tar.gz": "abc123",
	    "clang-tidy-linux-arm64.tar.gz": "def456"
	  }
	}`)
	cfg, err := parseConfig(data)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.Version != "v3.0.0" {
		t.Errorf("Version = %q, want v3.0.0", cfg.Version)
	}
	if cfg.DestDir != "clang" {
		t.Errorf("DestDir = %q, want clang", cfg.DestDir)
	}
	if cfg.Sha256["clang-tidy-linux-amd64.tar.gz"] != "abc123" {
		t.Errorf("sha256 lookup = %q, want abc123", cfg.Sha256["clang-tidy-linux-amd64.tar.gz"])
	}
}

func TestParseConfig_RejectsInvalidJSON(t *testing.T) {
	if _, err := parseConfig([]byte("not json")); err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestResolveURL(t *testing.T) {
	cfg := Config{
		Version: "v3.0.0",
		URL:     "https://host/clang-tidy/$version/$filename",
	}
	got := cfg.resolveURL("clang-tidy-linux-amd64.tar.gz")
	want := "https://host/clang-tidy/v3.0.0/clang-tidy-linux-amd64.tar.gz"
	if got != want {
		t.Errorf("resolveURL = %q, want %q", got, want)
	}
}
