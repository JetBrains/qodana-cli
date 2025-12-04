package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/JetBrains/qodana-cli/internal/platform/utils"
	log "github.com/sirupsen/logrus"
)

type Command struct {
	Directory string   `json:"directory"`
	Command   string   `json:"command"`
	File      string   `json:"file"`
	Output    string   `json:"output"`
	Arguments []string `json:"arguments,omitempty"`
}

type FileWithHeaders struct {
	File    string
	Headers []string
}

const (
	SIS = "#include <...> search starts here:"
	SIE = "End of search list."
)

func getHeaderType(file string) string {
	extension := filepath.Ext(file)
	nullDevice := os.DevNull
	switch extension {
	case ".c", ".h":
		return fmt.Sprintf("-E -Wp,-v -xc %s", nullDevice)
	default:
		return fmt.Sprintf("-E -Wp,-v -xc++ %s", nullDevice)
	}
}

// getFilesAndCompilers returns a list of files with their corresponding compiler's include directories
func getFilesAndCompilers(compileCommands string) ([]FileWithHeaders, error) {
	data, err := os.ReadFile(compileCommands)
	if err != nil {
		return nil, err
	}
	var commands []Command
	err = json.Unmarshal(data, &commands)
	if err != nil {
		return nil, err
	}
	var processList []FileWithHeaders
	fileHeaderMap := make(map[string][]string)

	for _, cmd := range commands {
		var compiler string
		trimmedCommand := strings.TrimSpace(cmd.Command)
		if trimmedCommand == "" {
			if cmd.Arguments == nil || len(cmd.Arguments) == 0 {
				log.Warn("Empty command and arguments for file in compilation db: ", cmd.File)
				continue
			}
			compiler = cmd.Arguments[0]
		} else {
			compiler = strings.Split(trimmedCommand, " ")[0]
		}
		headerType := getHeaderType(cmd.File)
		if val, ok := fileHeaderMap[compiler+headerType]; ok {
			processList = append(processList, FileWithHeaders{File: cmd.File, Headers: val})
		} else {
			headers, err := askCompiler(compiler, headerType)
			if err != nil {
				return nil, err
			}
			fileHeaderMap[compiler+headerType] = headers
			processList = append(processList, FileWithHeaders{File: cmd.File, Headers: headers})
		}
	}

	return processList, nil
}

// askCompiler asks the compiler for the include directories
func askCompiler(compiler string, headerType string) ([]string, error) {
	args := []string{compiler, headerType}
	_, stderr, _, err := utils.RunCmdRedirectOutput("", args...)
	if err != nil {
		return nil, err
	}
	startIndex := strings.Index(stderr, SIS)
	endIndex := strings.Index(stderr, SIE)
	var list []string
	if startIndex != -1 && endIndex != -1 && endIndex > startIndex {
		includes := strings.TrimSpace(stderr[startIndex+len(SIS) : endIndex])

		re := regexp.MustCompile(`[\n\r]+`)
		lines := re.Split(includes, -1)
		for _, dir := range lines {
			if strings.Contains(dir, "(") {
				continue
			}
			list = append(list, "--extra-arg=-isystem"+strings.TrimSpace(dir))
		}
	}

	log.Debug("Compiler: ", compiler, "Include dirs: ", list)
	return list, nil
}
