package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/JetBrains/qodana-cli/internal/foundation/exec"
	"github.com/JetBrains/qodana-cli/internal/foundation/shlex"
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

// compilerCacheKey produces an unambiguous cache key for the (compiler,
// headerType) pair. A null byte is used as the delimiter because it cannot
// appear in POSIX paths or shell arguments (and is also absent from Windows
// paths), so distinct inputs always produce distinct keys.
func compilerCacheKey(compiler string, headerType []string) string {
	var b strings.Builder
	b.WriteString(compiler)
	for _, h := range headerType {
		b.WriteByte(0)
		b.WriteString(h)
	}
	return b.String()
}

// pickCompiler returns the compiler binary name for a compile_commands.json
// Command entry. See pickCompilerFor for the parsing rules.
func pickCompiler(cmd Command) (string, bool) {
	return pickCompilerFor(cmd, runtime.GOOS)
}

// pickCompilerFor extracts the compiler (first token) from a compile_commands.json
// Command entry for the given GOOS. On Windows the command holds backslash-separated
// paths (CMake records e.g. C:\PROGRA~1\LLVM\bin\clang.exe), so it is tokenized with
// Windows quoting rules; POSIX shell splitting would strip the backslashes. Elsewhere
// the command is parsed as a POSIX shell line. If the command is empty or cannot be
// parsed, pickCompilerFor falls back to cmd.Arguments[0] when available; the second
// return is false when no compiler can be determined and the entry should be skipped.
func pickCompilerFor(cmd Command, goos string) (string, bool) {
	trimmed := strings.TrimSpace(cmd.Command)
	if trimmed == "" {
		if len(cmd.Arguments) == 0 {
			log.Warn("Empty command and arguments for file in compilation db: ", cmd.File)
			return "", false
		}
		return cmd.Arguments[0], true
	}

	if goos == "windows" {
		if tok, ok := firstWindowsToken(trimmed); ok {
			return tok, true
		}
		log.Warnf("Failed to parse command for file in compilation db %s", cmd.File)
		if len(cmd.Arguments) > 0 {
			return cmd.Arguments[0], true
		}
		return "", false
	}

	parts, err := shlex.Split(trimmed)
	if err != nil {
		log.Warnf("Failed to parse command for file in compilation db %s: %v", cmd.File, err)
		if len(cmd.Arguments) > 0 {
			return cmd.Arguments[0], true
		}
		return "", false
	}
	if len(parts) == 0 {
		if len(cmd.Arguments) > 0 {
			return cmd.Arguments[0], true
		}
		return "", false
	}
	return parts[0], true
}

// firstWindowsToken returns the first argument of a Windows command line: a
// double-quoted span (quotes stripped) or the run of characters up to the first
// whitespace. Unlike POSIX shell splitting it does not treat '\' as an escape, so
// backslash paths survive intact. ok is false for an unterminated quote so the
// caller can fall back to the arguments array.
func firstWindowsToken(command string) (string, bool) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", false
	}
	if command[0] == '"' {
		if end := strings.IndexByte(command[1:], '"'); end >= 0 {
			return command[1 : 1+end], true
		}
		return "", false // unterminated quote
	}
	if sp := strings.IndexAny(command, " \t"); sp >= 0 {
		return command[:sp], true
	}
	return command, true
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
		compiler, ok := pickCompiler(cmd)
		if !ok {
			continue
		}
		headerType := getHeaderType(cmd.File)
		cacheKey := compilerCacheKey(compiler, headerType)
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
		return nil, fmt.Errorf("compiler %q exited with code %d\n  stderr: %s",
			compiler, exitCode, strings.TrimRight(stderr, "\n"))
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
