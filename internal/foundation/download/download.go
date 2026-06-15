// Package download fetches a URL to a local file atomically, with optional bearer auth, optional
// checksum (any hash) verification, and optional progress reporting. It is the single download
// helper for build scripts and other headless callers (UI-free; callers adapt their own progress).
package download

import (
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/JetBrains/qodana-cli/internal/foundation/fs"
)

// Options configures ToFile. The zero value is a plain, unauthenticated, unverified GET.
type Options struct {
	// Client is the HTTP client to use; nil uses a default client with a 10-minute timeout for
	// large archives.
	Client *http.Client
	// Bearer, if non-empty, adds "Authorization: Bearer <Bearer>".
	Bearer string
	// Hash, if non-nil, is computed over the body; its lowercase-hex digest is returned.
	Hash func() hash.Hash
	// ExpectedHex, if non-empty (requires Hash), must equal the computed digest or ToFile errors
	// and leaves no file behind.
	ExpectedHex string
	// Progress, if non-nil, is called as bytes arrive with (downloaded, total); total is the
	// Content-Length or -1 when unknown.
	Progress func(downloaded, total int64)
}

var defaultClient = &http.Client{Timeout: 10 * time.Minute}

// ToFile downloads url to destPath atomically (temp file + rename, never a partial file), creating
// the parent directory if missing. It returns the lowercase-hex digest when Options.Hash is set
// (else ""). A non-200 response, an empty body (0 bytes), or an ExpectedHex mismatch is an error and
// destPath is left untouched.
func ToFile(url, destPath string, opts Options) (string, error) {
	if opts.ExpectedHex != "" && opts.Hash == nil {
		return "", fmt.Errorf("download: ExpectedHex set without Hash; cannot verify %s", url)
	}
	client := opts.Client
	if client == nil {
		client = defaultClient
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	if opts.Bearer != "" {
		req.Header.Set("Authorization", "Bearer "+opts.Bearer)
	}
	// Request identity encoding so the bytes we hash are exactly the artifact bytes: with Go's default
	// transparent gzip, a proxy serving Content-Encoding: gzip would make the digest not match the pin.
	req.Header.Set("Accept-Encoding", "identity")
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
	w, err := fs.CreateAtomic(destPath, 0o644)
	if err != nil {
		return "", err
	}

	var hasher hash.Hash
	dst := io.Writer(w)
	if opts.Hash != nil {
		hasher = opts.Hash()
		dst = io.MultiWriter(w, hasher)
	}
	src := io.Reader(resp.Body)
	if opts.Progress != nil {
		src = &progressReader{r: resp.Body, total: resp.ContentLength, cb: opts.Progress}
	}

	n, err := io.Copy(dst, src)
	if err != nil {
		return "", errors.Join(fmt.Errorf("download %s: %w", url, err), w.Abort())
	}
	if n == 0 {
		return "", errors.Join(fmt.Errorf("download %s: empty response (0 bytes)", url), w.Abort())
	}

	digest := ""
	if hasher != nil {
		digest = hex.EncodeToString(hasher.Sum(nil))
		if opts.ExpectedHex != "" && digest != opts.ExpectedHex {
			return "", errors.Join(
				fmt.Errorf("%s: checksum mismatch: got %s, expected %s", filepath.Base(destPath), digest, opts.ExpectedHex),
				w.Abort())
		}
	}
	if err := w.Close(); err != nil { // commit
		return "", err
	}
	return digest, nil
}

// progressReader reports bytes read so far to a callback. total is the Content-Length or -1.
type progressReader struct {
	r     io.Reader
	total int64
	done  int64
	cb    func(downloaded, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.done += int64(n)
		p.cb(p.done, p.total)
	}
	return n, err
}
