package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// CheckForUpdates check GitHub https://github.com/JetBrains/qodana-cli/ for the latest version of CLI release.
func CheckForUpdates(currentVersion string) {
	go func() {
		resp, err := http.Get(releaseUrl)
		if err != nil {
			logrus.Errorf("Failed to check for updates: %s", err)
			return
		}
		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				logrus.Errorf("Failed to close response body: %s", err)
				return
			}
		}(resp.Body)
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			logrus.Errorf("Failed to check for updates: %s", resp.Status)
			return
		}
		bodyText, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			logrus.Errorf("Failed to read response body: %s", err)
			return
		}
		result := make(map[string]interface{})
		err = json.Unmarshal(bodyText, &result)
		if err != nil {
			logrus.Errorf("Failed to read response JSON: %s", err)
			return
		}
		latestVersion := result["tag_name"].(string)
		if latestVersion != fmt.Sprintf("v%s", currentVersion) {
			WarningMessage(
				"New version of %s is available: %s. See https://jb.gg/qodana-cli/update\n   Set %s=1 environment variable to never get this message again\n",
				PrimaryBold("qodana"),
				latestVersion,
				SkipCheckForUpdateEnv,
			)
		}
	}()
}

// openReport serves the report on the given port and opens the browser.
func openReport(path string, port int) {
	url := fmt.Sprintf("http://localhost:%d", port)
	go func() {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == 200 {
			err := openBrowser(url)
			if err != nil {
				return
			}
		}
	}()
	http.Handle("/", noCache(http.FileServer(http.Dir(path))))
	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		WarningMessage("Problem serving report, %s\n", err.Error())
		return
	}
	_, _ = fmt.Scan()
}

// openBrowser opens the default browser to the given url
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

// OpenDir opens directory in the default file manager
func OpenDir(path string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "explorer"
		args = []string{"/select"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, path)
	return exec.Command(cmd, args...).Start()
}

// noCache handles serving the static files with no cache headers.
func noCache(h http.Handler) http.Handler {
	etagHeaders := []string{
		"ETag",
		"If-Modified-Since",
		"If-Match",
		"If-None-Match",
		"If-Range",
		"If-Unmodified-Since",
	}
	epoch := time.Unix(0, 0).Format(time.RFC1123)
	noCacheHeaders := map[string]string{
		"Expires":         epoch,
		"Cache-Control":   "no-cache, private, max-age=0",
		"Pragma":          "no-cache",
		"X-Accel-Expires": "0",
	}
	fn := func(w http.ResponseWriter, r *http.Request) {
		for _, v := range etagHeaders {
			if r.Header.Get(v) != "" {
				r.Header.Del(v)
			}
		}
		for k, v := range noCacheHeaders {
			w.Header().Set(k, v)
		}
		h.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

// getProjectId returns the project id for internal CLI usage from the given path.
func getProjectId(project string) string {
	projectAbs, _ := filepath.Abs(project)
	sha256sum := sha256.Sum256([]byte(projectAbs))
	return hex.EncodeToString(sha256sum[:])
}

// GetLinterSystemDir returns path to <userCacheDir>/JetBrains/<linter>/<project-id>/.
func GetLinterSystemDir(project string, linter string) string {
	userCacheDir, _ := os.UserCacheDir()
	linterDirName := strings.Replace(strings.Replace(linter, ":", "-", -1), "/", "-", -1)

	return filepath.Join(
		userCacheDir,
		"JetBrains",
		linterDirName,
		getProjectId(project),
	)
}

// Contains checks if a string is in a given slice.
func Contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

// Append appends a string to a slice if it's not already there.
func Append(slice []string, elems ...string) []string {
	if !Contains(slice, elems[0]) {
		slice = append(slice, elems[0])
	}
	return slice
}
