package main

import (
	"fmt"
	"os"
	"os/signal"
	"path"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JetBrains/qodana-cli/internal/foundation/exec"
	"github.com/JetBrains/qodana-cli/internal/foundation/fs"
	"github.com/JetBrains/qodana-cli/internal/platform"
	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	"github.com/briandowns/spinner"
	log "github.com/sirupsen/logrus"
)

const spinnerIndex = 34
const spinnerInterval = 100 * time.Millisecond

// runClangTidyUnderProgress runs clang-tidy for each file in filesAndCompilers and shows a progress bar.
// configFile, when non-empty, is passed to clang-tidy via --config-file=.
// extraClangArgs is the already-parsed user --clang-args (see prepareClangArgs).
func runClangTidyUnderProgress(
	c thirdpartyscan.Context,
	filesAndCompilers []FileWithHeaders,
	checks string,
	configFile string,
	extraClangArgs []string,
) {
	spin := initializeSpinner()
	stdoutChannel, stderrChannel := createFileLoggers(c.LogDir())
	worker(c, filesAndCompilers, checks, configFile, extraClangArgs, spin, stdoutChannel, stderrChannel)
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
		if err := fs.AppendToFile(fileName, logItem); err != nil {
			log.Error(err)
		}
	}
}

func worker(
	c thirdpartyscan.Context,
	filesAndCompilers []FileWithHeaders,
	checks string,
	configFile string,
	extraClangArgs []string,
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
						configFile,
						extraClangArgs,
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

// runClangTidy runs clang-tidy for a single file.
//
// configFile, when non-empty, is forwarded as --config-file=<path>. It is
// inserted before the extraClangArgs splice so that a user-supplied
// --config-file= appearing earlier than a `--` separator in extraClangArgs
// wins (clang-tidy's --config-file is a cl::opt — last occurrence wins).
// Tokens after `--` are forwarded to the compiler and do not reach
// clang-tidy's own option parser.
//
// extraClangArgs is the already-parsed user --clang-args (see
// prepareClangArgs). It is appended verbatim to the argv.
func runClangTidy(
	counter int,
	input FileWithHeaders,
	checks string,
	configFile string,
	extraClangArgs []string,
	c thirdpartyscan.Context,
	tmpResultsDir string,
	stderrChannel chan string,
	stdoutChannel chan string,
) error {
	clangPath := c.ClangPath()
	var args []string
	if checks != "" {
		args = append(args, checks)
	}
	if configFile != "" {
		args = append(args, "--config-file="+configFile)
	}
	args = append(args,
		"-p",
		c.ClangCompileCommands(),
		"--export-sarif",
		path.Join(tmpResultsDir, fmt.Sprintf("%d.sarif.json", counter)),
	)
	args = append(args, input.Headers...)
	args = append(args, input.File)
	args = append(args, "--quiet")
	args = append(args, extraClangArgs...)
	stdout, stderr, _, err := exec.ExecRedirectOutput(
		c.ProjectDir(),
		clangPath, args...,
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
