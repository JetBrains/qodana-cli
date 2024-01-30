package core

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/shirou/gopsutil/v3/process"
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"strings"
)

// findProcess using gopsutil to find process by name.
func findProcess(processName string) bool {
	if platform.IsContainer() {
		return isProcess(processName)
	}
	p, err := process.Processes()
	if err != nil {
		log.Fatal(err)
	}
	for _, proc := range p {
		name, err := proc.Name()
		if err == nil {
			if name == processName {
				return true
			}
		}
	}
	return false
}

// isProcess returns true if a process with cmd containing 'find' substring exists.
func isProcess(find string) bool {
	processes, err := process.Processes()
	if err != nil {
		return false
	}
	for _, proc := range processes {
		cmd, err := proc.Cmdline()
		if err != nil {
			continue
		}
		if strings.Contains(cmd, find) {
			return true
		}
	}
	return false
}

// isInstalled checks if git is installed.
func isInstalled(what string) bool {
	help := ""
	if what == "git" {
		help = ", refer to https://git-scm.com/downloads for installing it"
	}

	_, err := exec.LookPath(what)
	if err != nil {
		platform.WarningMessage(
			"Unable to find %s"+help,
			what,
		)
		return false
	}
	return true
}

// createUser will make dynamic uid as a valid user `idea`, needed for gradle cache.
func createUser(fn string) {
	if //goland:noinspection ALL
	os.Getuid() == 0 {
		return
	}
	idea := fmt.Sprintf("idea:x:%d:%d:idea:/root:/bin/bash", os.Getuid(), os.Getgid())
	data, err := os.ReadFile(fn)
	if err != nil {
		log.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		if line == idea {
			return
		}
	}
	if err = os.WriteFile(fn, []byte(strings.Join(append(lines, idea), "\n")), 0o777); err != nil {
		log.Fatal(err)
	}
}

func writeFileIfNew(filepath string, content string) {
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		if err := os.WriteFile(filepath, []byte(content), 0o755); err != nil {
			log.Fatal(err)
		}
	}
}

func getPluginIds(plugins []platform.Plugin) []string {
	ids := make([]string, len(plugins))
	for i, plugin := range plugins {
		ids[i] = plugin.Id
	}
	return ids
}

func (o *QodanaOptions) guessProduct() string {
	if o.Ide != "" {
		productCode := strings.TrimSuffix(o.Ide, EapSuffix)
		if _, ok := Products[productCode]; ok {
			return productCode
		}
		return ""
	} else if o.Linter != "" {
		// if Linter contains registry.jetbrains.team/p/sa/containers/ or https://registry.jetbrains.team/p/sa/containers/
		// then replace it with jetbrains/ and do the comparison
		linter := strings.TrimPrefix(o.Linter, "https://")
		if strings.HasPrefix(linter, "registry.jetbrains.team/p/sa/containers/") {
			linter = strings.TrimPrefix(linter, "registry.jetbrains.team/p/sa/containers/")
			linter = "jetbrains/" + linter
		}
		for k, v := range platform.DockerImageMap {
			if strings.HasPrefix(linter, v) {
				return k
			}
		}
	}
	return ""
}
