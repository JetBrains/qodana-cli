//go:build ignore

package main

import (
	"log"
	"os"
	"os/exec"
)

// go generate -v -x ./...
func main() {
	log.Println("Running go generate -v -x ./...")
	command := exec.Command("go", "generate", "-v", "-x", "./...")
	command.Stderr = os.Stderr
	command.Stdout = os.Stdout
	if err := command.Run(); err != nil {
		log.Fatalf("Error while running go generate: %s", err)
	}
}
