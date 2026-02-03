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
	log "github.com/sirupsen/logrus"
)

type Publisher struct {
	ResultsDir string
	LogDir     string
	AnalysisId string
}

// SendReport sends report to Qodana Cloud.
func SendReport(publisher Publisher, token string, javaPath string) {
	if javaPath == "" {
		log.Fatal(
			"Java is required to send reports to Qodana Cloud without linter execution. " +
				"See requirements in our documentation: https://www.jetbrains.com/help/qodana/deploy-qodana.html",
		)
	}
	file, err := os.CreateTemp("", "qodana-publisher.jar")
	if err != nil {
		log.Fatalf("Failed to create a temporary file: %s", err)
	}
	publisherPath := file.Name()
	err = file.Close()
	if err != nil {
		log.Fatalf("Failed to close temporary file %q: %s", file.Name(), err)
	}
	defer func() {
		err = os.Remove(file.Name())
		if err != nil {
			log.Fatalf("Failed to remove temporary file %q: %s", file.Name(), err)
		}
	}()

	extractPublisher(publisherPath)

	publisherCommand := getPublisherArgs(
		javaPath,
		publisherPath,
		publisher,
		token,
		cloud.GetCloudRootEndpoint().Url,
	)
	if _, _, res, err := utils.LaunchAndLog(publisher.LogDir, "publisher", publisherCommand...); res > 0 || err != nil {
		os.Exit(res)
	}
}

// getPublisherArgs returns args for the publisher.
func getPublisherArgs(java string, publisherPath string, publisher Publisher, token string, endpoint string) []string {
	publisherArgs := []string{
		java,
		"-jar",
		publisherPath,
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

func extractPublisher(path string) {
	file, err := os.Create(path)
	if err != nil {
		log.Fatalf("Error while creating %q: %s", path, err)
	}
	defer func() {
		err := file.Close()
		if err != nil {
			log.Fatalf("Error while closing %q: %s", path, err)
		}
	}()

	_, err = file.Write(tooling.PublisherCli)
	if err != nil {
		log.Fatalf("Error while writing %q: %s", path, err)
	}
}
