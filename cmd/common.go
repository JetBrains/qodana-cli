package cmd

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/tiulpin/qodana/pkg"
	"os"
	"os/exec"
)

// RunCommand runs the command
func RunCommand(cmd *exec.Cmd) {
	log.Info("running", cmd.String())
	if err := cmd.Run(); err != nil {
		log.Fatal("\nProblem occurred:", err.Error())
	}
}

// PrintProcess prints the message for processing phase
// 	TODO: Add ETA based on previous runs
func PrintProcess(f func(), what string) {
	if err := pkg.Spin(f, "Running project "+what); err != nil {
		log.Fatal("\nProblem occurred:", err.Error())
	}
	pkg.Primary.Println("✅  Finished project " + what)
}

// ensureDockerInstalled checks if docker is installed
// 	TODO: Windows support? Yes, we will use Docker API
func ensureDockerInstalled() {
	cmd := exec.Command("which", "docker")
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

// EnsureDockerRunning checks if docker daemon is running
func EnsureDockerRunning() {
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
