package downloaddeps

import (
	"fmt"
	"net/http"
	"os"
)

// processFile ensures destPath holds the correct bytes for filename and returns the SHA-256 now
// on disk.
//
//   - cache: in normal mode, if destPath already matches the pinned hash, nothing is downloaded.
//   - normal: the download must match cfg.Sha256[filename]; a mismatch is an error and destPath
//     is removed so a corrupt artifact is never left behind.
//   - force: the pin is ignored, the file is always downloaded, and the actual hash is returned
//     for the caller to write back into the config.
func processFile(client *http.Client, cfg Config, filename, token, destPath string, force bool) (string, error) {
	expected := cfg.Sha256[filename]

	if !force && expected != "" {
		if existing, err := sha256Hex(destPath); err == nil && existing == expected {
			return existing, nil
		}
	}

	sha, err := downloadFile(client, cfg.resolveURL(filename), token, destPath)
	if err != nil {
		return "", err
	}

	if force {
		return sha, nil
	}
	if sha != expected {
		_ = os.Remove(destPath)
		return "", fmt.Errorf("%s: sha256 mismatch: got %s, pinned %s (run with --force to update the pin)",
			filename, sha, expected)
	}
	return sha, nil
}
