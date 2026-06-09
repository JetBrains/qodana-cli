package downloaddeps

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestSha256Hex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.bin")
	content := []byte("hello qodana")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	want := hex.EncodeToString(sha256Sum(content))

	got, err := sha256Hex(path)
	if err != nil {
		t.Fatalf("sha256Hex: %v", err)
	}
	if got != want {
		t.Errorf("sha256Hex = %q, want %q", got, want)
	}
	if len(got) != 64 {
		t.Errorf("hex length = %d, want 64", len(got))
	}
}

func sha256Sum(b []byte) []byte {
	s := sha256.Sum256(b)
	return s[:]
}
