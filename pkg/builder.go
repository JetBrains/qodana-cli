package pkg

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"os/exec"
	"path/filepath"
)

type CommandBuilder interface {
	SetProjectDir(projectDir string)
	SetImageName(name string)
	SetShowReport(port string)
	SetSaveReport(path string)
	GetCommand() string
}

type DefaultBuilder struct {
	imageName           string
	dockerArguments     []string
	entryPointArguments []string
	reportShared        bool
}

func NewDefaultBuilder() *DefaultBuilder {
	return &DefaultBuilder{}
}

func (b *DefaultBuilder) GetCommand() *exec.Cmd {
	args := make([]string, 0)
	args = append(args, "run")
	args = append(args, b.dockerArguments...)
	args = append(args, b.imageName)
	args = append(args, b.entryPointArguments...)
	return exec.Command("docker", args...)
}

func (b *DefaultBuilder) SetImageName(name string) {
	b.imageName = name
}

func (b *DefaultBuilder) SetProjectDir(projectDir string) {
	b.dockerArguments = append(b.dockerArguments, getVolumeArg(projectDir, "/data/project")...)
}

func (b *DefaultBuilder) SetShowReport(port string) {
	b.dockerArguments = append(b.dockerArguments, "-p", fmt.Sprintf("%s:8080", port))
	b.entryPointArguments = append(b.entryPointArguments, "--show-report")
}

func (b *DefaultBuilder) SetSaveReport(path string) {
	b.dockerArguments = append(b.dockerArguments, getVolumeArg(path, "/data/results")...)
	b.entryPointArguments = append(b.entryPointArguments, "--save-report")
}

func (b *DefaultBuilder) SetCacheDir(path string) {
	b.dockerArguments = append(b.dockerArguments, getVolumeArg(path, "/data/cache")...)
}

func (b *DefaultBuilder) SetOptions(opt *LinterOptions) {
	b.SetImageName(opt.ImageName)
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
