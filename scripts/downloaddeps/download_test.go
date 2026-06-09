package downloaddeps

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadFile_WritesAndHashes(t *testing.T) {
	body := []byte("clang-tidy archive bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "nested", "clang-tidy-linux-amd64.tar.gz")
	sha, err := downloadFile(srv.Client(), srv.URL, "", dest)
	if err != nil {
		t.Fatalf("downloadFile: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("dest not written: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("dest content = %q, want %q", got, body)
	}
	if want := hex.EncodeToString(sha256Sum(body)); sha != want {
		t.Errorf("returned sha = %q, want %q", sha, want)
	}
}

func TestDownloadFile_SetsAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "f")
	if _, err := downloadFile(srv.Client(), srv.URL, "tok123", dest); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer tok123")
	}
}

func TestDownloadFile_NoAuthWhenEmpty(t *testing.T) {
	var hadAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadAuth = r.Header["Authorization"]
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "f")
	if _, err := downloadFile(srv.Client(), srv.URL, "", dest); err != nil {
		t.Fatal(err)
	}
	if hadAuth {
		t.Error("Authorization header present when token empty")
	}
}

func TestDownloadFile_ErrorsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "f")
	_, err := downloadFile(srv.Client(), srv.URL, "", dest)
	if err == nil {
		t.Fatal("expected error on 404, got nil")
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Error("dest file should not exist after failed download")
	}
}

func TestDownloadFile_RejectsZeroBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // empty body
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "f")
	_, err := downloadFile(srv.Client(), srv.URL, "", dest)
	if err == nil {
		t.Fatal("expected error on 0-byte response, got nil")
	}
}
