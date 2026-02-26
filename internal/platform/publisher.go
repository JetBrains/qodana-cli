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

/*
 * This file contains the code for sending the report to Qodana Cloud.
 * The publisher is a part of Qodana linters.
 * This will be refactored/removed after the proper endpoint is implemented.
 */

package platform

import (
	"os"

	"github.com/JetBrains/qodana-cli/internal/cloud"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/JetBrains/qodana-cli/internal/platform/utils"
	"github.com/JetBrains/qodana-cli/internal/tooling"
)

type Publisher struct {
	ResultsDir string
	LogDir     string
	AnalysisId string
}

// SendReport sends report to Qodana Cloud.
func SendReport(cacheDir string, publisher Publisher, token string) {

	publisherCommand := getPublisherArgs(
		cacheDir,
		publisher,
		token,
		cloud.GetCloudRootEndpoint().Url,
	)
	if _, _, res, err := utils.LaunchAndLog(publisher.LogDir, "publisher", publisherCommand...); res > 0 || err != nil {
		os.Exit(res)
	}
}

// getPublisherArgs returns args for the publisher.
func getPublisherArgs(cacheDir string, publisher Publisher, token string, endpoint string) []string {
	publisherArgs := []string{
		tooling.GetQodanaJBRPath(cacheDir),
		"-jar",
		tooling.PublisherCli.GetLibPath(cacheDir),
		"--analysis-id", publisher.AnalysisId,
		"--report-path", publisher.ResultsDir,
		"--token", token,
	}
	var tools []string
	tool := os.Getenv(qdenv.QodanaToolEnv)
	if tool != "" {
		tools = []string{tool}
	}
	if len(tools) > 0 {
		for _, t := range tools {
			publisherArgs = append(publisherArgs, "--tool", t)
		}
	}
	if endpoint != "" {
		publisherArgs = append(publisherArgs, "--qodana-endpoint", endpoint)
	}
	return publisherArgs
}
