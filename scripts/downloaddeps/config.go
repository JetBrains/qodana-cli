package downloaddeps

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Config is a per-dependency pin file (scripts/downloaddeps/<name>.json). It records the exact
// version to download, a URL template, the destination directory, and the expected SHA-256 of
// every file. Renovate bumps `version`; maintainers refresh `sha256` via the --force flow.
type Config struct {
	Version string            `json:"version"`
	URL     string            `json:"url"`
	DestDir string            `json:"dest_dir"`
	Sha256  map[string]string `json:"sha256"`
}

func parseConfig(data []byte) (Config, error) {
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return c, nil
}

// resolveURL expands the $version and $filename placeholders in the URL template.
func (c Config) resolveURL(filename string) string {
	r := strings.NewReplacer("$version", c.Version, "$filename", filename)
	return r.Replace(c.URL)
}
