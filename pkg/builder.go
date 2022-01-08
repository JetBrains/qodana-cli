package pkg

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"io/ioutil"
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

type QodanaYaml struct {
	Version string          `yaml:"version"`
	Linters []string        `yaml:"linters"`
	Exclude []QodanaExclude `yaml:"exclude"`
}

type QodanaExclude struct {
	Name  string   `yaml:"name"`
	Paths []string `yaml:"paths"`
}

func NewDefaultBuilder() *DefaultBuilder {
	return &DefaultBuilder{}
}

func (b *DefaultBuilder) GetCommand(opt *LinterOptions) *exec.Cmd {
	args := make([]string, 0)
	args = append(args, "run")
	args = append(args, b.dockerArguments...)
	linter := getQodanaYaml(opt.ProjectPath).Linters[0]
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

func getQodanaYaml(path string) *QodanaYaml {
	q := &QodanaYaml{}
	yamlFile, err := ioutil.ReadFile(filepath.Join(path, "qodana.yaml"))
	if err != nil {
		log.Printf("yamlFile.Get err   #%v ", err)
	}
	err = yaml.Unmarshal(yamlFile, q)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}
	return q
}

func getVolumeArg(srcPath string, tgtPath string) []string {
	absPath, err := filepath.Abs(srcPath)
	if err != nil {
		log.Fatal("couldn't get abs path for project: ", err.Error())
	}
	dockerArg := fmt.Sprintf("%s:%s", absPath, tgtPath)
	return []string{"-v", dockerArg}
}

func WriteQodanaYaml(path string, linters []string) {
	q := &QodanaYaml{
		Version: "1.0",
		Linters: linters,
		Exclude: []QodanaExclude{*&QodanaExclude{
			Name: "All",
			Paths: []string{
				".qodana/",
			},
		}},
	}
	yamlFile, err := yaml.Marshal(q)
	if err != nil {
		log.Fatalf("yamlFile.Write err   #%v ", err)
	}
	err = ioutil.WriteFile(filepath.Join(path, "qodana.yaml"), yamlFile, 0644)
	if err != nil {
		log.Fatalf("Marshal: %v", err)
	}
}
