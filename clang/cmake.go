package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/JetBrains/qodana-cli/internal/foundation/exec"
	"github.com/google/shlex"
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

// Markers in the compiler's -v stderr output that delimit the include search path list.
const (
	SIS = "#include <...> search starts here:"
	SIE = "End of search list."
)

// getHeaderType returns compiler flags that cause it to preprocess an empty
// file and print its include search paths. Uses -xc for C and -xc++ for C++.
func getHeaderType(file string) []string {
	nullDevice := os.DevNull
	switch filepath.Ext(file) {
	case ".c", ".h":
		return []string{"-E", "-Wp,-v", "-xc", nullDevice}
	default:
		return []string{"-E", "-Wp,-v", "-xc++", nullDevice}
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
			if len(cmd.Arguments) == 0 {
				log.Warn("Empty command and arguments for file in compilation db: ", cmd.File)
				continue
			}
			compiler = cmd.Arguments[0]
		} else {
			parts, err := shlex.Split(trimmedCommand)
			if err != nil || len(parts) == 0 {
				log.Warnf("Failed to parse command for file in compilation db: %s", cmd.File)
				continue
			}
			compiler = parts[0]
		}
		headerType := getHeaderType(cmd.File)
		cacheKey := compiler + strings.Join(headerType, " ")
		if val, ok := fileHeaderMap[cacheKey]; ok {
			processList = append(processList, FileWithHeaders{File: cmd.File, Headers: val})
		} else {
			headers, err := askCompiler(compiler, headerType)
			if err != nil {
				return nil, err
			}
			fileHeaderMap[cacheKey] = headers
			processList = append(processList, FileWithHeaders{File: cmd.File, Headers: headers})
		}
	}

	return processList, nil
}

// askCompiler retrieves the compiler's built-in system include directories by
// running it with `-E -Wp,-v -xc /dev/null` (or `-xc++` for C++ files).
// The -Wp,-v flag tells the preprocessor to print its search paths to stderr,
// delimited by "#include <...> search starts here:" and "End of search list.".
// Each discovered path is passed to clang-tidy as --extra-arg=-isystem<path>.
// See https://gcc.gnu.org/onlinedocs/gcc/Preprocessor-Options.html (-v flag).
func askCompiler(compiler string, headerType []string) ([]string, error) {
	// Force English output so the SIS/SIE markers are not translated by GCC's gettext.
	env := append(os.Environ(), "LC_ALL=C")
	_, stderr, exitCode, err := exec.ExecRedirectOutputWithEnv(".", env, compiler, headerType...)
	if err != nil {
		return nil, err
	}
	if exitCode != 0 {
		return nil, fmt.Errorf("compiler %q exited with code %d", compiler, exitCode)
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
