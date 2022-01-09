package cmd

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/tiulpin/qodana/pkg"
	"os"
	"os/exec"
)

// runCommand runs the command
func runCommand(cmd *exec.Cmd) {
	log.Info("running", cmd.String())
	if err := cmd.Run(); err != nil {
		log.Fatal("\nProblem occurred:", err.Error())
	}
}

// printProcess prints the message for processing phase. TODO: Add ETA based on previous runs
func printProcess(f func(), start string, finished string) {
	if err := pkg.Spin(f, start); err != nil {
		log.Fatal("\nProblem occurred:", err.Error())
	}
	pkg.Primary.Printfln("✅  Finished %s ", finished)
}

// ensureDockerInstalled checks if docker is installed
func ensureDockerInstalled() {
	cmd := exec.Command("docker", "--version")
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			pkg.Error.Println(
				"Docker is not installed on your system, ",
				"refer to https://www.docker.com/get-started for installing it",
			)
			os.Exit(1)
		}
		log.Fatal(err)
	}
}

// ensureDockerRunning checks if docker daemon is running
func ensureDockerRunning() {
	ensureDockerInstalled()
	cmd := exec.Command("docker", "ps")
	if err := cmd.Run(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			pkg.Error.Println(fmt.Sprintf(
				"Docker exited with exit code %d, perhaps docker daemon is not running?",
				exiterr.ExitCode(),
			))
			os.Exit(1)
		}
		log.Fatal(err)
	}
}
