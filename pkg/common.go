package pkg

import (
	"bufio"
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type QodanaOptions struct {
	ResultsDir            string
	CacheDir              string
	ProjectDir            string
	Linter                string
	SourceDirectory       string
	DisableSanity         bool
	ProfileName           string
	ProfilePath           string
	RunPromo              bool
	StubProfile           string
	Baseline              string
	BaselineIncludeAbsent bool
	SaveReport            bool
	ShowReport            bool
	Port                  int
	Property              string
	Script                string
	FailThreshold         string
	Changes               bool
	SendReport            bool
	Token                 string
	AnalysisId            string
	EnvVariables          []string
	UnveilProblems        bool
}

var Version = "0.5.1" // TODO: check for updates
var DoNotTrack = false
var Interrupted = false
var internalStages = []string{
	"Preparing Qodana Docker images",
	"Starting the analysis engine",
	"Opening the project",
	"Configuring the project",
	"Analyzing the project",
	"Preparing the report",
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

// CheckLinter validates the image used for the scan.
func CheckLinter(image string) {
	if !strings.HasPrefix(image, OfficialDockerPrefix) {
		WarningMessage("You are using an unofficial Qodana linter " + image + "\n")
		unofficialLinter = true
	}
	for _, linter := range notSupportedLinters {
		if linter == image {
			log.Fatalf("%s is not supported by Qodana CLI", linter)
		}
	}
}

// ConfigureProject sets up the project directory for Qodana CLI to run
// Looks up .idea directory to determine used modules
// If a project doesn't have .idea, then runs language detector
func ConfigureProject(projectDir string) {
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
	languages := ReadIdeaFolder(projectDir)
	if len(languages) == 0 {
		languages, _ = RecognizeDirLanguages(projectDir)
	}
	for _, language := range languages {
		if linter, err := langLinters[language]; err {
			if !Contains(linters, linter) {
				linters = append(linters, linter)
			}
		}
	}
	if len(linters) == 0 {
		ErrorMessage("Qodana does not support this project yet. See https://www.jetbrains.com/help/qodana/supported-technologies.html")
		os.Exit(1)
	}
	WriteQodanaYaml(projectDir, linters)
	SuccessMessage(fmt.Sprintf("Added %s", PrimaryBold.Sprint(linters[0])))
}

// GetLinterHome returns path to <project>/.qodana/<linter>/
func GetLinterHome(project string, linter string) string {
	dotQodana := filepath.Join(project, ".qodana")
	parentDirName := strings.Replace(strings.Replace(linter, ":", "-", -1), "/", "-", -1)
	return filepath.Join(dotQodana, parentDirName)
}

// PrepareFolders cleans up report folder, creates the necessary folders for the analysis
func PrepareFolders(opts *QodanaOptions) {
	linterHome := GetLinterHome(opts.ProjectDir, opts.Linter)
	if opts.ResultsDir == "" {
		opts.ResultsDir = filepath.Join(linterHome, "results")
	}
	if opts.CacheDir == "" {
		opts.CacheDir = filepath.Join(linterHome, "cache")
	}
	if _, err := os.Stat(opts.ResultsDir); err == nil {
		err := os.RemoveAll(opts.ResultsDir)
		if err != nil {
			return
		}
	}
	if err := os.MkdirAll(opts.CacheDir, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
	if err := os.MkdirAll(opts.ResultsDir, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
}

// ShowReport serves the Qodana report
func ShowReport(path string, port int) { // TODO: Open report from Cloud
	PrintProcess(
		func() {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				log.Fatal("Qodana report not found. Get the report by running `qodana scan`")
			}
			openReport(path, port)
		},
		fmt.Sprintf("Showing Qodana report at http://localhost:%d, press Ctrl+C to stop", port),
		"",
	)
}

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
	http.Handle("/", http.FileServer(http.Dir(path)))
	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		WarningMessage(fmt.Sprintf("Problem serving report, %s\n", err.Error()))
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

func RunLinter(ctx context.Context, options *QodanaOptions) int64 {
	docker, err := client.NewClientWithOpts()
	if err != nil {
		log.Fatal("couldn't instantiate docker client", err)
	}
	for i, stage := range internalStages {
		internalStages[i] = PrimaryBold.Sprintf("[%d/%d] ", i+1, len(internalStages)+1) + Primary.Sprint(stage)
	}
	CheckLinter(options.Linter)
	progress, _ := StartQodanaSpinner(internalStages[0])
	pullImage(ctx, docker, options.Linter)
	dockerOpts := getDockerOptions(options)
	tryRemoveContainer(ctx, docker, dockerOpts.Name)
	updateText(progress, internalStages[1])
	runContainer(ctx, docker, dockerOpts)

	reader, _ := docker.ContainerLogs(context.Background(), dockerOpts.Name, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: false,
	})
	defer func(reader io.ReadCloser) {
		err := reader.Close()
		if err != nil {
			log.Fatal(err.Error())
		}
	}(reader)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		if !unofficialLinter && strings.Contains(line, "By using this Docker image") {
			WarningMessage(licenseWarning(line, options.Linter))
		}
		if strings.Contains(line, "Starting up") {
			updateText(progress, internalStages[2])
		}
		if strings.Contains(line, "The Project opening stage completed in") {
			updateText(progress, internalStages[3])
		}
		if strings.Contains(line, "The Project configuration stage completed in") {
			updateText(progress, internalStages[4])
		}
		if strings.Contains(line, "---- Qodana - Detailed summary ----") {
			updateText(progress, internalStages[5])
			break
		}
	}
	exitCode := getDockerExitCode(ctx, docker, dockerOpts.Name)
	_ = progress.Stop()
	return exitCode
}
