package downloaddeps

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func serverReturning(body []byte, hits *int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			*hits++
		}
		_, _ = w.Write(body)
	}))
}

func TestProcessFile_CacheHitSkipsDownload(t *testing.T) {
	body := []byte("cached bytes")
	sum := hex.EncodeToString(sha256Sum(body))
	dest := filepath.Join(t.TempDir(), "clt.zip")
	if err := os.WriteFile(dest, body, 0o644); err != nil {
		t.Fatal(err)
	}

	hits := 0
	srv := serverReturning(body, &hits)
	defer srv.Close()
	cfg := Config{Version: "v1", URL: srv.URL + "/$filename", Sha256: map[string]string{"clt.zip": sum}}

	sha, err := processFile(srv.Client(), cfg, "clt.zip", "", dest, false)
	if err != nil {
		t.Fatalf("processFile: %v", err)
	}
	if hits != 0 {
		t.Errorf("expected no download on cache hit, got %d hits", hits)
	}
	if sha != sum {
		t.Errorf("sha = %q, want %q", sha, sum)
	}
}

func TestProcessFile_DownloadsAndVerifies(t *testing.T) {
	body := []byte("fresh bytes")
	sum := hex.EncodeToString(sha256Sum(body))
	srv := serverReturning(body, nil)
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "clt.zip")
	cfg := Config{Version: "v1", URL: srv.URL + "/$filename", Sha256: map[string]string{"clt.zip": sum}}

	if _, err := processFile(srv.Client(), cfg, "clt.zip", "", dest, false); err != nil {
		t.Fatalf("processFile: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != string(body) {
		t.Errorf("dest = %q, want %q", got, body)
	}
}

func TestProcessFile_VerifyMismatchErrors(t *testing.T) {
	body := []byte("actual bytes")
	srv := serverReturning(body, nil)
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "clt.zip")
	cfg := Config{Version: "v1", URL: srv.URL + "/$filename", Sha256: map[string]string{"clt.zip": "deadbeef"}}

	_, err := processFile(srv.Client(), cfg, "clt.zip", "", dest, false)
	if err == nil {
		t.Fatal("expected sha mismatch error, got nil")
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Error("dest should not remain after verification failure")
	}
}

func TestProcessFile_ForceReturnsActualShaIgnoringPin(t *testing.T) {
	body := []byte("forced bytes")
	want := hex.EncodeToString(sha256Sum(body))
	srv := serverReturning(body, nil)
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "clt.zip")
	cfg := Config{Version: "v1", URL: srv.URL + "/$filename", Sha256: map[string]string{"clt.zip": "stale"}}

	sha, err := processFile(srv.Client(), cfg, "clt.zip", "", dest, true)
	if err != nil {
		t.Fatalf("force processFile: %v", err)
	}
	if sha != want {
		t.Errorf("force returned sha %q, want actual %q", sha, want)
	}
}
