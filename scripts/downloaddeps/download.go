package downloaddeps

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// downloadFile GETs url (with an optional Bearer token), streams it to destPath atomically,
// and returns the SHA-256 of the downloaded bytes. A non-200 response or an empty body is an
// error, and destPath is left untouched in either case.
func downloadFile(client *http.Client, url, token, destPath string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("get %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("get %s: HTTP %d: %s", url, resp.StatusCode, body)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(destPath), ".download-*.tmp")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), resp.Body)
	if closeErr := tmp.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	if n == 0 {
		return "", fmt.Errorf("download %s: empty response (0 bytes)", url)
	}

	if err := os.Rename(tmp.Name(), destPath); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
