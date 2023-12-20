package core

import (
	"github.com/JetBrains/qodana-cli/v2023/platform"
	log "github.com/sirupsen/logrus"
	"path/filepath"
)

// SetQodanaLinter adds the linter to the qodana.yaml file.
func SetQodanaLinter(path string, linter string, filename string) {
	q := platform.LoadQodanaYaml(path, filename)
	if q.Version == "" {
		q.Version = "1.0"
	}
	q.Sort()
	if platform.Contains(AllCodes, linter) {
		q.Ide = linter
	} else {
		q.Linter = linter
	}
	err := q.WriteConfig(filepath.Join(path, filename))
	if err != nil {
		log.Fatalf("writeConfig: %v", err)
	}
}

// setQodanaDotNet adds the .NET configuration to the qodana.yaml file.
func setQodanaDotNet(path string, dotNet *platform.DotNet, filename string) bool {
	q := platform.LoadQodanaYaml(path, filename)
	q.DotNet = *dotNet
	err := q.WriteConfig(filepath.Join(path, filename))
	if err != nil {
		log.Fatalf("writeConfig: %v", err)
	}
	return true
}
