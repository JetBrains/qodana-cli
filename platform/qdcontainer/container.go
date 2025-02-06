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

package qdcontainer

import (
	"context"
	"errors"
	"github.com/JetBrains/qodana-cli/v2024/platform/msg"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdenv"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func checkRequiredToolInstalled(tool string) bool {
	_, err := exec.LookPath(tool)
	return err == nil
}

func PrepareContainerEnvSettings() {
	var tool string
	if os.Getenv(qdenv.QodanaCliUsePodman) == "" && checkRequiredToolInstalled("docker") {
		tool = "docker"
	} else if checkRequiredToolInstalled("podman") {
		tool = "podman"
	} else {
		msg.ErrorMessage(
			"Docker (or podman) is not installed on the system or can't be found in PATH, refer to https://www.docker.com/get-started for installing it",
		)
		os.Exit(1)
	}
	cmd := exec.Command(tool, "ps")
	if err := cmd.Run(); err != nil {
		var exiterr *exec.ExitError
		if errors.As(err, &exiterr) {
			if strings.Contains(string(exiterr.Stderr), "permission denied") {
				msg.ErrorMessage(
					"Qodana container can't be run by the current user. Please fix the container engine configuration.",
				)
				msg.WarningMessage("https://docs.docker.com/engine/install/linux-postinstall/#manage-docker-as-a-non-root-user")
				os.Exit(1)
			} else {
				msg.ErrorMessage(
					"'%s ps' exited with exit code %d, perhaps docker daemon is not running?",
					tool,
					exiterr.ExitCode(),
				)
			}
			os.Exit(1)
		}
		log.Fatal(err)
	}
	CheckContainerEngineMemory()
}

// CheckContainerEngineMemory applicable only for Docker Desktop,
// (has the default limit of 2GB which can be not enough when Gradle runs inside a container).
func CheckContainerEngineMemory() {
	docker := GetContainerClient()
	goos := runtime.GOOS
	if //goland:noinspection GoBoolExpressions
	goos != "windows" && goos != "darwin" {
		return
	}
	info, err := docker.Info(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	var helpUrl string
	switch goos {
	case "windows":
		helpUrl = "https://docs.docker.com/desktop/settings/windows/#advanced"
	case "darwin":
		helpUrl = "https://docs.docker.com/desktop/settings/mac/#advanced-1"
	}
	log.Debug("Docker memory limit is set to ", info.MemTotal/1024/1024, " MB")

	if info.MemTotal < 4*1024*1024*1024 {
		msg.WarningMessage(
			`The container daemon is running with less than 4GB of RAM.
   If you experience issues, consider increasing the container runtime memory limit.
   Refer to %s for more information.
`,
			helpUrl,
		)
	}
}

// GetContainerClient getContainerClient returns a docker client.
func GetContainerClient() *client.Client {
	docker, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		log.Fatal("couldn't create container client ", err)
	}
	return docker
}
