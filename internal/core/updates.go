/*
 * Copyright 2021-2024 JetBrains s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	cienvironment "github.com/cucumber/ci-environment/go"

	"github.com/JetBrains/qodana-cli/internal/platform/msg"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
)

var DisableCheckUpdates = false

var (
	releaseURL  = "https://api.github.com/repos/JetBrains/qodana-cli/releases/latest"
	startOnce   sync.Once
	printOnce   sync.Once
	done        = make(chan struct{})
	updateMsg   string
	httpTimeout = 10 * time.Second
	cacheTTL    = 24 * time.Hour
	cacheDir    = defaultCacheDir
)

type updateCache struct {
	Version   string    `json:"version"`
	CheckedAt time.Time `json:"checked_at"`
}

type releaseResponse struct {
	TagName string `json:"tag_name"`
}

func defaultCacheDir() string {
	dir, _ := os.UserCacheDir()
	return filepath.Join(dir, "JetBrains", "Qodana")
}

func StartUpdateCheck(currentVersion string) {
	startOnce.Do(func() {
		go func() {
			defer close(done)
			if shouldSkipCheck(currentVersion) {
				return
			}
			latest := resolveLatestVersion()
			if latest == "" {
				return
			}
			if compareSemver(currentVersion, latest) < 0 {
				updateMsg = fmt.Sprintf(
					"New version of %s CLI is available: %s. See https://jb.gg/qodana-cli/update\n",
					msg.PrimaryBold("qodana"),
					latest,
				)
			}
		}()
	})
}

func PrintUpdateNotice() {
	printOnce.Do(func() {
		if DisableCheckUpdates {
			return
		}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			return
		}
		if updateMsg != "" {
			msg.WarningMessage(updateMsg)
		}
	})
}

var shouldSkipCheck = defaultShouldSkipCheck

func defaultShouldSkipCheck(v string) bool {
	return v == "dev" ||
		strings.HasSuffix(v, "nightly") ||
		qdenv.IsContainer() ||
		cienvironment.DetectCIEnvironment() != nil
}

func resolveLatestVersion() string {
	if v, ok := readCache(); ok {
		return v
	}
	v := fetchLatestVersion()
	if v != "" {
		writeCache(v)
	}
	return v
}

func fetchLatestVersion() string {
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(releaseURL)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return ""
	}
	var release releaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return ""
	}
	if release.TagName == "" {
		return ""
	}
	return strings.TrimPrefix(release.TagName, "v")
}

func cacheFilePath() string {
	return filepath.Join(cacheDir(), "update-check.json")
}

func readCache() (string, bool) {
	data, err := os.ReadFile(cacheFilePath())
	if err != nil {
		return "", false
	}
	var c updateCache
	if err := json.Unmarshal(data, &c); err != nil {
		return "", false
	}
	if time.Since(c.CheckedAt) > cacheTTL {
		return "", false
	}
	return c.Version, true
}

func writeCache(version string) {
	c := updateCache{
		Version:   version,
		CheckedAt: time.Now(),
	}
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	dir := cacheDir()
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(cacheFilePath(), data, 0o644)
}

func compareSemver(a, b string) int {
	av := parseSemver(a)
	bv := parseSemver(b)
	if av == nil || bv == nil {
		return 0
	}
	for i := 0; i < 3; i++ {
		if av[i] < bv[i] {
			return -1
		}
		if av[i] > bv[i] {
			return 1
		}
	}
	return 0
}

func parseSemver(v string) []int {
	v = strings.TrimPrefix(v, "v")
	if idx := strings.IndexByte(v, '-'); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nil
	}
	result := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		result[i] = n
	}
	return result
}

func resetUpdateCheck() {
	startOnce = sync.Once{}
	printOnce = sync.Once{}
	done = make(chan struct{})
	updateMsg = ""
	shouldSkipCheck = defaultShouldSkipCheck
}
