package main

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2025/platform"
	"github.com/JetBrains/qodana-cli/v2025/platform/strutil"
	"github.com/JetBrains/qodana-cli/v2025/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
	"github.com/briandowns/spinner"
	log "github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"path"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const spinnerIndex = 34
const spinnerInterval = 100 * time.Millisecond

// runClangTidyUnderProgress runs clang-tidy for each file in filesAndCompilers and shows a progress bar.
func runClangTidyUnderProgress(c thirdpartyscan.Context, filesAndCompilers []FileWithHeaders, checks string) {
	spin := initializeSpinner()
	stdoutChannel, stderrChannel := createFileLoggers(c.LogDir())
	worker(c, filesAndCompilers, checks, spin, stdoutChannel, stderrChannel)
}

func initializeSpinner() *spinner.Spinner {
	spin := spinner.New(spinner.CharSets[9], spinnerInterval, spinner.WithWriter(os.Stderr))
	spin.UpdateCharSet(spinner.CharSets[spinnerIndex])
	return spin
}

func createFileLoggers(logDir string) (chan string, chan string) {
	stdout := make(chan string)
	stderr := make(chan string)

	go logToFile(path.Join(logDir, "clang-out.txt"), stdout)
	go logToFile(path.Join(logDir, "clang-err.txt"), stderr)

	return stdout, stderr
}

func logToFile(fileName string, logChannel chan string) {
	for logItem := range logChannel {
		if err := utils.AppendToFile(fileName, logItem); err != nil {
			log.Error(err)
		}
	}
}

func worker(
	c thirdpartyscan.Context,
	filesAndCompilers []FileWithHeaders,
	checks string,
	spin *spinner.Spinner,
	stdoutChannel, stderrChannel chan string,
) {
	spin.Start()
	spin.Suffix = fmt.Sprintf(" %d/%d", 0, len(filesAndCompilers))

	defer close(stdoutChannel)
	defer close(stderrChannel)

	var wg sync.WaitGroup
	progressCounter := int32(0)
	sem := make(chan bool, runtime.NumCPU())
	quit := make(chan os.Signal, 1)
	finished := make(chan bool)
	signal.Notify(quit, os.Interrupt)

	testClangTidyArch(c)
	go func() {
		for counter, fileWithHeader := range filesAndCompilers {
			select {
			case <-quit:
				fmt.Println("Interrupt signal received. Exiting function.")
				spin.Stop()
				finished <- true
				return
			default:
				wg.Add(1)
				go func(counter int, input FileWithHeaders) {
					defer wg.Done()
					sem <- true
					spin.Suffix = fmt.Sprintf(" %d/%d", atomic.AddInt32(&progressCounter, 1), len(filesAndCompilers))
					spin.Restart()
					defer func() { <-sem }()

					err := runClangTidy(
						counter,
						input,
						checks,
						c,
						platform.GetTmpResultsDir(c.ResultsDir()),
						stderrChannel,
						stdoutChannel,
					)
					if err != nil {
						log.Errorf("Error running clang-tidy: %s", err)
					}
				}(counter, fileWithHeader)
			}
		}
		wg.Wait()
		spin.Stop()
		finished <- true
	}()
	select {
	case <-quit:
		return
	case <-finished:
		return
	}
}

func testClangTidyArch(
	c thirdpartyscan.Context,
) {
	args := []string{
		"/usr/share/file",
		strutil.QuoteIfSpace(c.ClangPath()),
	}

	stdout, stderr, _, _ := utils.RunCmdRedirectOutput(
		strutil.QuoteIfSpace(c.ProjectDir()),
		args...,
	)

	if stderr != "" {
		log.Debug(stderr)
	}
	if stdout != "" {
		log.Debug(stdout)
	}
}

// runClangTidy runs clang-tidy for a single file.
func runClangTidy(
	counter int,
	input FileWithHeaders,
	checks string,
	c thirdpartyscan.Context,
	tmpResultsDir string,
	stderrChannel chan string,
	stdoutChannel chan string,
) error {
	args := []string{
		strutil.QuoteIfSpace(c.ClangPath()),
		checks,
		"-p",
		strutil.QuoteIfSpace(c.ClangCompileCommands()),
		"--export-sarif",
		strutil.QuoteIfSpace(path.Join(tmpResultsDir, fmt.Sprintf("%d.sarif.json", counter))),
	}
	args = append(args, input.Headers...)
	args = append(args, input.File)
	args = append(args, "--quiet")
	for _, arg := range strings.Split(c.ClangArgs(), " ") {
		args = append(args, arg)
	}
	stdout, stderr, _, err := utils.RunCmdRedirectOutput(
		strutil.QuoteIfSpace(c.ProjectDir()),
		args...,
	)
	if stderr != "" {
		log.Debug(stderr)
		stderrChannel <- fmt.Sprintf("File: %s\n%s\n", input.File, stderr)
	}
	if stdout != "" {
		log.Debug(stdout)
		stdoutChannel <- fmt.Sprintf("File: %s\n%s\n%s\n", input.File, stdout, stderr)
	}
	return err
}
