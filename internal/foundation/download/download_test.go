package download

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serve(t *testing.T, body []byte, capture *string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capture != nil {
			*capture = r.Header.Get("Authorization")
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestToFile_PlainGetReturnsEmptyDigestAndWritesFile(t *testing.T) {
	body := []byte("payload")
	dest := filepath.Join(t.TempDir(), "out.bin")
	sum, err := ToFile(serve(t, body, nil), dest, Options{})
	require.NoError(t, err)
	assert.Equal(t, "", sum, "no Hash means no digest")
	got, _ := os.ReadFile(dest)
	assert.Equal(t, body, got)
}

func TestToFile_Sha256ReturnsHexAndSendsBearer(t *testing.T) {
	body := []byte("payload")
	var auth string
	dest := filepath.Join(t.TempDir(), "out.bin")
	sum, err := ToFile(serve(t, body, &auth), dest, Options{Bearer: "tok", Hash: sha256.New})
	require.NoError(t, err)
	want := sha256.Sum256(body)
	assert.Equal(t, hex.EncodeToString(want[:]), sum)
	assert.Equal(t, "Bearer tok", auth)
}

func TestToFile_ExpectedHexMismatchErrorsAndLeavesNoFile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out.bin")
	_, err := ToFile(serve(t, []byte("payload"), nil), dest, Options{Hash: sha256.New, ExpectedHex: "deadbeef"})
	require.Error(t, err)
	_, statErr := os.Stat(dest)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestToFile_Sha512Verifies(t *testing.T) {
	body := []byte("payload")
	want := sha512.Sum512(body)
	dest := filepath.Join(t.TempDir(), "out.bin")
	sum, err := ToFile(serve(t, body, nil), dest, Options{Hash: sha512.New, ExpectedHex: hex.EncodeToString(want[:])})
	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(want[:]), sum)
}

func TestToFile_Non200ErrorsAndLeavesNoFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "out.bin")
	_, err := ToFile(srv.URL, dest, Options{})
	require.Error(t, err)
	_, statErr := os.Stat(dest)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestToFile_EmptyBodyErrorsAndLeavesNoFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "out.bin")
	_, err := ToFile(srv.URL, dest, Options{})
	require.Error(t, err)
	_, statErr := os.Stat(dest)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestToFile_ExpectedHexWithoutHashIsError(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out.bin")
	_, err := ToFile(serve(t, []byte("payload"), nil), dest, Options{ExpectedHex: "abc"})
	require.Error(t, err, "ExpectedHex without Hash must be rejected, not silently unverified")
}

func TestToFile_ProgressReceivesTotals(t *testing.T) {
	// Body < 4 KiB so the test server sets Content-Length (no chunked encoding), letting us assert total.
	body := make([]byte, 2000)
	dest := filepath.Join(t.TempDir(), "out.bin")
	var lastDone, lastTotal int64
	_, err := ToFile(serve(t, body, nil), dest, Options{Progress: func(done, total int64) { lastDone, lastTotal = done, total }})
	require.NoError(t, err)
	assert.Equal(t, int64(len(body)), lastDone)
	assert.Equal(t, int64(len(body)), lastTotal, "Content-Length is reported as total")
}
