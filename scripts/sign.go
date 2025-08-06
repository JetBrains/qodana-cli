//go:build ignore

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"slices"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: go run sign.go <PROJECT-NAME>")
	}

	project := os.Args[1]

	targetOs := runtime.GOOS
	if override := os.Getenv("TARGETOS"); override != "" {
		targetOs = override
	}

	targetArch := runtime.GOARCH
	if override := os.Getenv("TARGETARCH"); override != "" {
		targetArch = override
	}

	if targetOs == "linux" {
		log.Println("Signing skipped; not required on linux")
		return
	}

	if os.Getenv("SIGN") != "true" {
		log.Println("Signing skipped; set environment variable SIGN=true to enable signing")
		return
	}

	var dirSuffix string
	switch targetArch {
	case "amd64":
		dirSuffix = "_v1/"
	case "arm64":
		dirSuffix = "_v8.0/"
	default:
		log.Fatalf("Unsupported architecture '%s'", targetArch)
	}

	var exeExtension string
	var extraArgs []string
	switch targetOs {
	case "windows":
		exeExtension = ".exe"
		extraArgs = []string{}
	case "darwin":
		exeExtension = ""
		extraArgs = []string{"-denoted-content-type=application/x-mac-app-bin"}
	default:
		log.Fatalf("Unsupported OS '%s'", targetOs)
	}

	dir := fmt.Sprintf("./dist/%s_%s_%s%s", project, targetOs, targetArch, dirSuffix)
	exe := fmt.Sprintf("%sqodana-clang%s", dir, exeExtension)

	command := slices.Concat([]string{"codesign"}, extraArgs, []string{
		"-signed-files-dir", dir,
		exe,
	})

	log.Printf("Running %v", command)
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}
