package pkg

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"path/filepath"
)

// ensureDockerInstalled checks if docker is installed
func ensureDockerInstalled() {
	cmd := exec.Command("docker", "--version")
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			Error.Println(
				"Docker is not installed on your system, ",
				"refer to https://www.docker.com/get-started for installing it",
			)
			os.Exit(1)
		}
		log.Fatal(err)
	}
}

// EnsureDockerRunning checks if docker daemon is running
func EnsureDockerRunning() {
	ensureDockerInstalled()
	cmd := exec.Command("docker", "ps")
	if err := cmd.Run(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			Error.Println(fmt.Sprintf(
				"Docker exited with exit code %d, perhaps docker daemon is not running?",
				exiterr.ExitCode(),
			))
			os.Exit(1)
		}
		log.Fatal(err)
	}
}

type DockerCommandBuilder interface {
	SetProjectDir(projectDir string)
	SetSaveReport(path string)
	GetCommand() string
}

type DefaultBuilder struct {
	dockerArguments     []string
	entryPointArguments []string
}

func (b *DefaultBuilder) GetDockerCommand(opt *LinterOptions, linter string) *exec.Cmd {
	args := make([]string, 0)
	args = append(args, "run")
	args = append(args, "--rm")
	args = append(args, "--pull", "always")
	args = append(args, b.dockerArguments...)
	args = append(args, linter)
	args = append(args, b.entryPointArguments...)
	return exec.Command("docker", args...)
}

func (b *DefaultBuilder) SetProjectDir(projectDir string) {
	b.dockerArguments = append(b.dockerArguments, getVolumeArg(projectDir, "/data/project")...)
}

func (b *DefaultBuilder) SetSaveReport(path string) {
	b.dockerArguments = append(b.dockerArguments, getVolumeArg(path, "/data/results")...)
	b.entryPointArguments = append(b.entryPointArguments, "--save-report")
}

func (b *DefaultBuilder) SetCacheDir(path string) {
	b.dockerArguments = append(b.dockerArguments, getVolumeArg(path, "/data/cache")...)
}

func (b *DefaultBuilder) SetOptions(opt *LinterOptions) {
	b.SetSaveReport(opt.ResultsDir)
	b.SetCacheDir(opt.CachePath)
	b.SetProjectDir(opt.ProjectDir)
}

func getVolumeArg(srcPath string, tgtPath string) []string {
	absPath, err := filepath.Abs(srcPath)
	if err != nil {
		log.Fatal("couldn't get abs path for project: ", err.Error())
	}
	dockerArg := fmt.Sprintf("%s:%s", absPath, tgtPath)
	return []string{"-v", dockerArg}
}
