package pkg

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type LinterOptions struct {
	ResultsDir string
	CachePath  string
	ProjectDir string
}

// Contains checks if a string is in a given slice
func Contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

func ConfigureProject(options *LinterOptions) {
	var linters []string
	version := "2021.3-eap"
	langLinters := map[string]string{
		"Java":       fmt.Sprintf("jetbrains/qodana-jvm:%s", version),
		"Kotlin":     fmt.Sprintf("jetbrains/qodana-jvm:%s", version),
		"Python":     fmt.Sprintf("jetbrains/qodana-python:%s", version),
		"PHP":        fmt.Sprintf("jetbrains/qodana-php:%s", version),
		"JavaScript": fmt.Sprintf("jetbrains/qodana-js:%s", version),
		"TypeScript": fmt.Sprintf("jetbrains/qodana-js:%s", version),
	}
	languages := ReadIdeaFolder(options.ProjectDir)
	if len(languages) == 0 {
		languages, _ = RecognizeDirLanguages(options.ProjectDir)
	}
	for _, language := range languages {
		if linter, err := langLinters[language]; err {
			if !Contains(linters, linter) {
				linters = append(linters, linter)
			}
		}
	}
	path, _ := filepath.Abs(options.ProjectDir)
	if len(linters) == 0 {
		Error.Printfln(
			"Qodana does not support the project %s yet. See https://www.jetbrains.com/help/qodana/supported-technologies.html",
			path,
		)
		os.Exit(1)
	} else {
		for _, linter := range linters {
			Primary.Printfln("- Added %s", linter)
		}
	}
	WriteQodanaYaml(options.ProjectDir, linters)
}

// PrepareFolders cleans up report folder, creates the necessary folders for the analysis
func PrepareFolders(opts *LinterOptions) {
	if _, err := os.Stat(opts.ResultsDir); err == nil {
		err := os.RemoveAll(opts.ResultsDir)
		if err != nil {
			return
		}
	}
	if err := os.MkdirAll(opts.CachePath, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
	if err := os.MkdirAll(opts.ResultsDir, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
}

// ShowReport serves the Qodana report
func ShowReport(path string, port int) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Fatal("Qodana report not found. Get the report by running `qodana scan`")
	}
	url := fmt.Sprintf("http://localhost:%d", port)
	go func() {
		err := openBrowser(url)
		if err != nil {
			log.Fatal(err.Error())
		}
	}()
	http.Handle("/", http.FileServer(http.Dir(path)))
	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		return
	}
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
