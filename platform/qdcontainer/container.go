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
	"fmt"
	"os"
	"runtime"

	"github.com/JetBrains/qodana-cli/v2025/platform/msg"
	"github.com/docker/cli/cli/command"
	dockerCliConfig "github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

const (
	MountDir             = "/data/project"
	DataResultsDir       = "/data/results"
	DataResultsReportDir = "/data/results/report"
	DataCacheDir         = "/data/cache"
	DataCoverageDir      = "/data/coverage"
	DataGlobalConfigDir  = "/data/qodana-global-config/" // when container is launched by CLI, qodana-global-configurations.yaml file is mounted here
)

func PrepareContainerEnvSettings() {
	ctx := context.Background()
	_, err := NewContainerClient(ctx)
	if err != nil {
		msg.ErrorMessage(
			"An error occured while connecting to Docker: %s\n"+
				"Make sure that Docker or Podman is installed and a socket is available. If Docker is already "+
				"running, consider setting DOCKER_HOST variable explicitly.",
			err,
		)
		os.Exit(1)
	}

	checkEngineMemory()
}

// checkEngineMemory applicable only for Docker Desktop,
// (has the default limit of 2GB which can be not enough when Gradle runs inside a container).
func checkEngineMemory() {
	docker, err := NewContainerClient(context.Background())
	if err != nil {
		log.Fatal(err)
	}

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
	//goland:noinspection GoDfaConstantCondition
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

// NewContainerClient getContainerClient returns a docker client.
func NewContainerClient(ctx context.Context) (client.APIClient, error) {
	logWarnWriter := log.StandardLogger().WriterLevel(log.WarnLevel)
	configFile := dockerCliConfig.LoadDefaultConfigFile(logWarnWriter)
	err := logWarnWriter.Close()
	if err != nil {
		log.Warnf("Failed to close log writer: %s", err)
	}
	clientOptions := flags.NewClientOptions()

	apiClient, err := command.NewAPIClientFromFlags(clientOptions, configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker API client: %w", err)
	}
	apiClient.NegotiateAPIVersion(ctx)

	// A succesfull call to info is an indication that the client has connected to the socket successfully.
	info, err := apiClient.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Docker API: %w", err)
	}

	logClientInfo(info)

	return apiClient, nil
}

func logClientInfo(info system.Info) {
	if log.GetLevel() < log.DebugLevel {
		return
	}

	marshalledInfo, err := yaml.Marshal(info)
	if err != nil {
		log.Errorf("Failed to print info from Docker API: %s", err)
		return
	}

	log.Debugf("Docker API client info:\n%s", marshalledInfo)
}
