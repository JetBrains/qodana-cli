/*
 * Copyright 2021-2023 JetBrains s.r.o.
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

package core

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"github.com/JetBrains/qodana-cli/v2023/cloud"
	cp "github.com/otiai10/copy"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

type metadata struct {
	Versioning versioning `xml:"versioning"`
}

type versioning struct {
	Latest  string `xml:"latest"`
	Release string `xml:"release"`
}

// SendReport sends report to Qodana Cloud.
func SendReport(opts *QodanaOptions, token string) {
	path := Prod.IdeBin()
	if !IsContainer() {
		path = opts.ConfDirPath()
		fetchPublisher(path)
	}
	publisher := filepath.Join(path, "publisher.jar")
	if _, err := os.Stat(publisher); os.IsNotExist(err) {
		log.Fatalf("Not able to send the report: %s is missing", publisher)
	}
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

	publisherCommand := getPublisherArgs(Prod.JbrJava(), publisher, opts, token, os.Getenv(cloud.DefaultEndpoint))
	if res := RunCmd("", publisherCommand...); res > 0 {
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
		publisherArgs = append(publisherArgs, "--endpoint", endpoint)
	}
	return publisherArgs
}

func publisherVersion() versioning {
	resp, err := http.Get("https://packages.jetbrains.team/maven/p/ij/intellij-dependencies/org/jetbrains/qodana/publisher/maven-metadata.xml")
	if err != nil {
		log.Fatal(err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(resp.Body)

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	meta := &metadata{}
	err = xml.Unmarshal(content, meta)
	if err != nil {
		log.Fatal(err)
	}
	return meta.Versioning
}

func getPublisherUrl(version string) string {
	return "https://packages.jetbrains.team/maven/p/ij/intellij-dependencies/org/jetbrains/qodana/publisher-cli/" + version + "/publisher-cli-" + version + ".jar"
}

func fetchPublisher(directory string) {
	version := publisherVersion().Release
	path := filepath.Join(directory, "publisher.jar")
	if _, err := os.Stat(path); err == nil {
		return
	}
	err := DownloadFile(path, getPublisherUrl(version), nil)
	if err != nil {
		log.Fatal(err)
	}
	verifyMd5Hash(version, path)
}

func verifyMd5Hash(version string, path string) {
	if _, err := os.Stat(path); err != nil {
		log.Fatal(err)
	}
	url := getPublisherUrl(version) + ".md5"
	resp, err := http.Get(url)
	if err != nil {
		log.Fatalf("Error downloading md5 hash: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading md5 hash: %v", err)
	}

	downloadedMd5 := string(body)
	fileContent, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Error reading file: %v", err)
	}

	hasher := md5.New()
	_, err = hasher.Write(fileContent)
	if err != nil {
		log.Fatalf("Error computing md5 hash: %v", err)
	}

	computedMd5 := hex.EncodeToString(hasher.Sum(nil))

	if computedMd5 != downloadedMd5 {
		err = os.Remove(path)
		if err != nil {
			log.Fatalf("Please remove file, since md5 doesn't match: %s", path)
		}
		log.Fatal("The provided file and the file from the link have different md5 hashes")
	} else {
		println("Obtained publisher " + version + " and successfully checked md5 hash")
	}
}
