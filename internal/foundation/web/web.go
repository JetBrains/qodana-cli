package web

import (
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imroc/req/v3"
)

// NewClient creates a req.Client with sensible defaults: the given timeout and a User-Agent header.
func NewClient(timeout time.Duration) *req.Client {
	return req.C().
		SetTimeout(timeout).
		SetUserAgent("qodana-cli")
}

// Download streams url to destPath using atomic write (.part → rename).
// Creates parent directories as needed.
func Download(client *req.Client, url, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", destPath, err)
	}

	tmp := destPath + ".part"
	resp, err := client.R().SetOutputFile(tmp).Get(url)
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	if resp.GetStatusCode() != 200 {
		os.Remove(tmp)
		return fmt.Errorf("downloading %s: HTTP %d", url, resp.GetStatusCode())
	}

	if err := os.Rename(tmp, destPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming %s → %s: %w", tmp, destPath, err)
	}
	return nil
}

// DownloadAndVerify downloads url to destPath with atomic write (.part → rename),
// computing a hash during streaming and verifying against expectedHex.
// The newHash parameter specifies the hash algorithm (e.g. sha512.New, sha256.New).
func DownloadAndVerify(client *req.Client, url, destPath, expectedHex string, newHash func() hash.Hash) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", destPath, err)
	}

	resp, err := client.R().DisableAutoReadResponse().Get(url)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.GetStatusCode() != 200 {
		return fmt.Errorf("downloading %s: HTTP %d", url, resp.GetStatusCode())
	}

	tmp := destPath + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("creating %s: %w", tmp, err)
	}

	hasher := newHash()
	_, copyErr := io.Copy(io.MultiWriter(out, hasher), resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", tmp, copyErr)
	}
	if closeErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("closing %s: %w", tmp, closeErr)
	}

	actualHex := hex.EncodeToString(hasher.Sum(nil))
	if actualHex != expectedHex {
		os.Remove(tmp)
		return fmt.Errorf("hash mismatch for %s: expected %s, got %s", filepath.Base(url), expectedHex, actualHex)
	}

	if err := os.Rename(tmp, destPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming %s → %s: %w", tmp, destPath, err)
	}
	return nil
}

// ParseChecksumLine parses a "<hex>  <filename>" line and returns the hex portion.
// Validates that hex has expectedLen characters and is valid hex.
func ParseChecksumLine(line string, expectedLen int) (string, error) {
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) == 0 {
		return "", fmt.Errorf("empty checksum line")
	}
	hex_ := parts[0]
	if len(hex_) != expectedLen {
		return "", fmt.Errorf("expected %d hex chars, got %d (%q)", expectedLen, len(hex_), hex_)
	}
	if _, err := hex.DecodeString(hex_); err != nil {
		return "", fmt.Errorf("invalid hex: %w", err)
	}
	return hex_, nil
}
