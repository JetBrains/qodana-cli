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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCompareSemver(t *testing.T) {
	assert.Equal(t, 0, compareSemver("1.0.0", "1.0.0"))
	assert.Equal(t, -1, compareSemver("1.0.0", "2.0.0"))
	assert.Equal(t, 1, compareSemver("2.0.0", "1.0.0"))
	assert.Equal(t, -1, compareSemver("1.0.0", "1.0.1"))
	assert.Equal(t, 0, compareSemver("v1.0.0", "1.0.0"))
	assert.Equal(t, 0, compareSemver("1.0.0-beta", "1.0.0"))
	assert.Equal(t, 0, compareSemver("abc", "1.0.0"), "unparseable defaults to equal (no notification)")
}

func TestFetchLatestVersion(t *testing.T) {
	origURL := releaseURL
	defer func() { releaseURL = origURL }()

	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"tag_name": "v2.5.1"}`))
		}))
		defer srv.Close()
		releaseURL = srv.URL
		assert.Equal(t, "2.5.1", fetchLatestVersion())
	})

	t.Run("missing tag_name does not panic", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"name": "release"}`))
		}))
		defer srv.Close()
		releaseURL = srv.URL
		assert.Equal(t, "", fetchLatestVersion())
	})

	t.Run("server error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		releaseURL = srv.URL
		assert.Equal(t, "", fetchLatestVersion())
	})
}

func TestCacheFreshHitSkipsHTTP(t *testing.T) {
	origURL, origCacheDir := releaseURL, cacheDir
	defer func() { releaseURL = origURL; cacheDir = origCacheDir }()

	dir := t.TempDir()
	cacheDir = func() string { return dir }
	writeCache("3.0.0")

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()
	releaseURL = srv.URL

	assert.Equal(t, "3.0.0", resolveLatestVersion())
	assert.False(t, called)
}

func TestCacheStaleTriggersHTTP(t *testing.T) {
	origURL, origCacheDir := releaseURL, cacheDir
	defer func() { releaseURL = origURL; cacheDir = origCacheDir }()

	dir := t.TempDir()
	cacheDir = func() string { return dir }
	data, _ := json.Marshal(updateCache{Version: "1.0.0", CheckedAt: time.Now().Add(-48 * time.Hour)})
	_ = os.WriteFile(filepath.Join(dir, "update-check.json"), data, 0o644)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name": "v4.0.0"}`))
	}))
	defer srv.Close()
	releaseURL = srv.URL

	assert.Equal(t, "4.0.0", resolveLatestVersion())
}

func TestStartUpdateCheck(t *testing.T) {
	origURL, origCacheDir := releaseURL, cacheDir
	defer func() { releaseURL = origURL; cacheDir = origCacheDir }()

	neverSkip := func(string) bool { return false }

	newServer := func(tag string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"tag_name": "` + tag + `"}`))
		}))
	}

	t.Run("newer version sets message", func(t *testing.T) {
		resetUpdateCheck()
		shouldSkipCheck = neverSkip
		cacheDir = func() string { return t.TempDir() }
		srv := newServer("v9.9.9")
		defer srv.Close()
		releaseURL = srv.URL
		StartUpdateCheck("1.0.0")
		<-done
		assert.Contains(t, updateMsg, "9.9.9")
	})

	t.Run("same version no message", func(t *testing.T) {
		resetUpdateCheck()
		shouldSkipCheck = neverSkip
		cacheDir = func() string { return t.TempDir() }
		srv := newServer("v1.0.0")
		defer srv.Close()
		releaseURL = srv.URL
		StartUpdateCheck("1.0.0")
		<-done
		assert.Empty(t, updateMsg)
	})

	t.Run("dev skips fetch entirely", func(t *testing.T) {
		resetUpdateCheck()
		// use default shouldSkipCheck so CI/dev detection is active
		called := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
		defer srv.Close()
		releaseURL = srv.URL
		StartUpdateCheck("dev")
		<-done
		assert.False(t, called)
	})

	t.Run("DisableCheckUpdates suppresses print", func(t *testing.T) {
		resetUpdateCheck()
		shouldSkipCheck = neverSkip
		DisableCheckUpdates = false
		cacheDir = func() string { return t.TempDir() }
		srv := newServer("v9.9.9")
		defer srv.Close()
		releaseURL = srv.URL
		StartUpdateCheck("1.0.0")
		<-done
		DisableCheckUpdates = true
		PrintUpdateNotice() // must not panic
		DisableCheckUpdates = false
	})
}
