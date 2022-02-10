package core

import (
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

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
