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
	cp "github.com/otiai10/copy"
	"os"
	"path/filepath"

	"github.com/JetBrains/qodana-cli/v2024/cloud"
	"github.com/JetBrains/qodana-cli/v2024/tooling"
	log "github.com/sirupsen/logrus"
)

// SendReport sends report to Qodana Cloud.
func SendReport(opts *QodanaOptions, token string, javaPath string) {
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

	if !IsContainer() {
		if _, err := os.Stat(opts.ReportResultsPath()); os.IsNotExist(err) {
			if err := os.MkdirAll(opts.ReportResultsPath(), os.ModePerm); err != nil {
				log.Fatalf("failed to create directory: %v", err)
			}
		}
		source := filepath.Join(opts.ResultsDir, "qodana.sarif.json")
		destination := filepath.Join(opts.ReportResultsPath(), "qodana.sarif.json")

		if err := cp.Copy(source, destination); err != nil {
			log.Fatal(err)
		}
	}

	publisherCommand := getPublisherArgs(javaPath, publisherPath, opts, token, cloud.GetCloudRootEndpoint().Host)
	if _, _, res, err := LaunchAndLog(opts, "publisher", publisherCommand...); res > 0 || err != nil {
		os.Exit(res)
	}
}

// getPublisherArgs returns args for the publisher.
func getPublisherArgs(java string, publisher string, opts *QodanaOptions, token string, endpoint string) []string {
	publisherArgs := []string{
		QuoteForWindows(java),
		"-jar",
		QuoteForWindows(publisher),
		"--analysis-id", opts.AnalysisId,
		"--sources-path", QuoteForWindows(opts.ProjectDir),
		"--report-path", QuoteForWindows(opts.ReportResultsPath()),
		"--token", token,
	}
	var tools []string
	tool := os.Getenv(QodanaToolEnv)
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
