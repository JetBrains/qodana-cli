/*
 * Copyright 2021-2024 JetBrains s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package core

import (
	"github.com/JetBrains/qodana-cli/v2024/platform"
	startup "github.com/JetBrains/qodana-cli/v2024/preparehost"
	"github.com/shirou/gopsutil/v3/process"
	log "github.com/sirupsen/logrus"
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

// IsInstalled checks if git is installed.
func IsInstalled(what string) bool {
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

func getPluginIds(plugins []platform.Plugin) []string {
	ids := make([]string, len(plugins))
	for i, plugin := range plugins {
		ids[i] = plugin.Id
	}
	return ids
}

func GuessProductCode(ide string, linter string) string {
	if ide != "" {
		productCode := strings.TrimSuffix(ide, startup.EapSuffix)
		if _, ok := startup.Products[productCode]; ok {
			return productCode
		}
		return ""
	} else if linter != "" {
		// if Linter contains registry.jetbrains.team/p/sa/containers/ or https://registry.jetbrains.team/p/sa/containers/
		// then replace it with jetbrains/ and do the comparison
		linter := strings.TrimPrefix(linter, "https://")
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
