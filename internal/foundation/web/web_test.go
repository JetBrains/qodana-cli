package web

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	client := NewClient(5 * time.Second)
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
}

func TestDownload(t *testing.T) {
	content := "hello world download test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(content))
	}))
	defer server.Close()

	client := NewClient(10 * time.Second)
	dest := filepath.Join(t.TempDir(), "downloaded.txt")

	if err := Download(client, server.URL, dest); err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(data) != content {
		t.Fatalf("content mismatch: got %q, want %q", string(data), content)
	}

	// .part file should not exist
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Fatal(".part file should not exist after successful download")
	}
}

func TestDownloadHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(10 * time.Second)
	dest := filepath.Join(t.TempDir(), "should-not-exist.txt")

	err := Download(client, server.URL, dest)
	if err == nil {
		t.Fatal("expected error for HTTP 404")
	}

	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatal("file should not exist after failed download")
	}
	if _, statErr := os.Stat(dest + ".part"); !os.IsNotExist(statErr) {
		t.Fatal(".part file should not exist after failed download")
	}
}

func TestDownloadAndVerifySHA512(t *testing.T) {
	content := []byte("verified download content")
	hasher := sha512.New()
	hasher.Write(content)
	expectedHex := hex.EncodeToString(hasher.Sum(nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer server.Close()

	client := NewClient(10 * time.Second)
	dest := filepath.Join(t.TempDir(), "verified.bin")

	if err := DownloadAndVerify(client, server.URL, dest, expectedHex, sha512.New); err != nil {
		t.Fatalf("DownloadAndVerify (SHA-512) failed: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("content mismatch")
	}
}

func TestDownloadAndVerifySHA256(t *testing.T) {
	content := []byte("sha256 verified content")
	hasher := sha256.New()
	hasher.Write(content)
	expectedHex := hex.EncodeToString(hasher.Sum(nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer server.Close()

	client := NewClient(10 * time.Second)
	dest := filepath.Join(t.TempDir(), "verified-256.bin")

	if err := DownloadAndVerify(client, server.URL, dest, expectedHex, sha256.New); err != nil {
		t.Fatalf("DownloadAndVerify (SHA-256) failed: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("content mismatch")
	}
}

func TestDownloadAndVerifyMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("some content"))
	}))
	defer server.Close()

	client := NewClient(10 * time.Second)
	dest := filepath.Join(t.TempDir(), "should-not-exist.bin")
	wrongHex := hex.EncodeToString(make([]byte, 64)) // all zeros

	err := DownloadAndVerify(client, server.URL, dest, wrongHex, sha512.New)
	if err == nil {
		t.Fatal("expected error for hash mismatch")
	}

	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatal("file should not exist after hash mismatch")
	}
	if _, statErr := os.Stat(dest + ".part"); !os.IsNotExist(statErr) {
		t.Fatal(".part file should not exist after hash mismatch")
	}
}

func TestParseChecksumLine(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		expectedLen int
		wantHex     string
		wantErr     bool
	}{
		{
			name:        "valid SHA-512",
			line:        "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e  somefile.tar.gz",
			expectedLen: 128,
			wantHex:     "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e",
		},
		{
			name:        "valid SHA-256",
			line:        "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  emptyfile",
			expectedLen: 64,
			wantHex:     "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:        "hex only, no filename",
			line:        "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			expectedLen: 64,
			wantHex:     "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:        "wrong length",
			line:        "abcd1234  file.txt",
			expectedLen: 128,
			wantErr:     true,
		},
		{
			name:        "invalid hex",
			line:        "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz  file.txt",
			expectedLen: 64,
			wantErr:     true,
		},
		{
			name:        "empty line",
			line:        "",
			expectedLen: 128,
			wantErr:     true,
		},
		{
			name:        "whitespace only",
			line:        "   \t  ",
			expectedLen: 128,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseChecksumLine(tt.line, tt.expectedLen)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantHex {
				t.Fatalf("got %q, want %q", got, tt.wantHex)
			}
		})
	}
}
