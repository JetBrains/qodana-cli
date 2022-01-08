package pkg

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"os/exec"
	"path/filepath"
)

type CommandBuilder interface {
	SetProjectDir(projectDir string)
	SetSaveReport(path string)
	GetCommand() string
}

type DefaultBuilder struct {
	dockerArguments     []string
	entryPointArguments []string
	reportShared        bool
}

func NewDefaultBuilder() *DefaultBuilder {
	return &DefaultBuilder{}
}

func (b *DefaultBuilder) GetCommand(opt *LinterOptions, linter string) *exec.Cmd {
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
	b.SetSaveReport(opt.ReportPath)
	b.SetCacheDir(opt.CachePath)
	b.SetProjectDir(opt.ProjectPath)
}

func getVolumeArg(srcPath string, tgtPath string) []string {
	absPath, err := filepath.Abs(srcPath)
	if err != nil {
		log.Fatal("couldn't get abs path for project: ", err.Error())
	}
	dockerArg := fmt.Sprintf("%s:%s", absPath, tgtPath)
	return []string{"-v", dockerArg}
}
