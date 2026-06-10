package downloaddeps

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// writeConfig serializes a pin file deterministically (fixed struct field order; encoding/json sorts
// map keys) with a trailing newline, written atomically so a --force refresh never leaves a partial
// pin behind. Used only by the maintainer-facing --force flow.
func writeConfig(path string, cfg Config) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
