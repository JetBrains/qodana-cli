//go:build ignore

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"slices"
	"strings"
)

func run(name string, arg ...string) string {
	commandline := fmt.Sprintf("%v", slices.Insert(arg, 0, name))
	log.Printf("Running %v", commandline)
	command := exec.Command(name, arg...)
	command.Stderr = os.Stderr
	out, err := command.Output()

	if err != nil {
		log.Fatalf("Error while running %s: %s", commandline, err)
	}

	return string(out)
}

// go generate -v -x $(go list -m -f {{.Dir}} | xargs -I{} go list -e -find {}/...)
func main() {
	moduleDirs := run("go", "list", "-m", "-f", "{{.Dir}}")

	for moduleDir := range strings.SplitSeq(moduleDirs, "\n") {
		if moduleDir == "" {
			continue
		}

		run("go", "generate", "-v", "-x", fmt.Sprintf("%s/...", moduleDir))
	}
}
