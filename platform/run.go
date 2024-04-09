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

package platform

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/cloud"
	"github.com/JetBrains/qodana-cli/v2024/tooling"
	log "github.com/sirupsen/logrus"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// it's a 3rd party linter executor

func setup(options *QodanaOptions) error {
	linterOptions := options.GetLinterSpecificOptions()
	if linterOptions == nil {
		return fmt.Errorf("linter specific options are not set")
	}
	mountInfo := (*linterOptions).GetMountInfo()
	linterInfo := (*linterOptions).GetInfo(options)
	var err error

	mountInfo.JavaPath, err = getJavaExecutablePath()
	if err != nil {
		return fmt.Errorf("failed to get java executable path: %w", err)
	}
	// TODO iscommunityoreap
	cloud.SetupLicenseToken(options.GetToken())
	options.LicensePlan, err = cloud.GetCloudApiEndpoints().GetLicensePlan()
	if err != nil {
		if !linterInfo.IsEap {
			return fmt.Errorf("failed to get license plan: %w", err)
		}
		println("Qodana license plan: EAP license.")
		options.LicensePlan = cloud.CommunityLicensePlan
	}

	options.ResultsDir, err = filepath.Abs(options.ResultsDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path to results directory: %w", err)
	}
	options.ReportDir, err = filepath.Abs(options.reportDirPath())
	if err != nil {
		return fmt.Errorf("failed to get absolute path to report directory: %w", err)
	}
	tmpResultsDir := options.GetTmpResultsDir()
	// cleanup tmpResultsDir if it exists
	if _, err := os.Stat(tmpResultsDir); err == nil {
		if err := os.RemoveAll(tmpResultsDir); err != nil {
			return fmt.Errorf("failed to remove folder with temporary data: %w", err)
		}
	}

	directories := []string{
		options.ResultsDir,
		options.LogDirPath(),
		options.CacheDir,
		options.ReportResultsPath(),
		tmpResultsDir,
	}
	for _, dir := range directories {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		}
	}

	err = (*linterOptions).Setup(options)
	if err != nil {
		return fmt.Errorf("failed to setup linter specific options: %w", err)
	}

	return nil
}

func cleanup() {
	umount()
}

func RunAnalysis(options *QodanaOptions) (int, error) {
	// we don't provide default for cache dir, since we don't want to compute options.id without knowing the exact folder
	if options.CacheDir == "" {
		options.CacheDir = options.GetCacheDir()
	}
	if options.ResultsDir == "" {
		options.ResultsDir = options.resultsDirPath()
	}
	linterOptions := options.GetLinterSpecificOptions()
	if linterOptions == nil {
		ErrorMessage("linter specific options are not set")
		return 1, nil
	}
	mountInfo := (*linterOptions).GetMountInfo()
	linterInfo := (*linterOptions).GetInfo(options)
	err := setup(options)
	if err != nil {
		ErrorMessage(err.Error())
		return 1, err
	}
	options.LogOptions()
	printQodanaLogo(options, linterInfo)
	deviceId := GetDeviceIdSalt()[0] // TODO : let's move it to QodanaOptions
	defer cleanup()
	mount(options)
	events := make([]tooling.FuserEvent, 0)
	eventsCh := createFuserEventChannel(&events)
	defer sendFuserEvents(eventsCh, &events, options, deviceId)
	logOs(eventsCh)
	logProjectOpen(eventsCh)

	err = (*linterOptions).RunAnalysis(options)
	if err != nil {
		ErrorMessage(err.Error())
		return 1, err
	}
	log.Debugf("Java executable path: %s", mountInfo.JavaPath)
	var analysisResult int
	if analysisResult, err = computeBaselinePrintResults(options, mountInfo); err != nil {
		ErrorMessage(err.Error())
		return 1, err
	}
	if err = copySarifToReportPath(options); err != nil {
		ErrorMessage(err.Error())
		return 1, err
	}
	if err = convertReportToCloudFormat(options, mountInfo); err != nil {
		ErrorMessage(err.Error())
		return 1, err
	}
	if err = copyQodanaYamlToReportPath(options); err != nil {
		ErrorMessage(err.Error())
		return 1, err
	}
	sendReportToQodanaServer(options, mountInfo)
	logProjectClose(eventsCh)
	return analysisResult, nil
}

func sendReportToQodanaServer(options *QodanaOptions, mountInfo *MountInfo) {
	if cloud.Token.Token != "" {
		fmt.Println("Publishing report ...")
		SendReport(options, cloud.Token.Token, QuoteForWindows(filepath.Join(options.CacheDir, PublisherJarName)), QuoteForWindows(mountInfo.JavaPath))
	} else {
		fmt.Println("License token is not set, skipping report publishing")
	}
}

func copyQodanaYamlToReportPath(options *QodanaOptions) error {
	if yamlPath, err := GetQodanaYamlPath(options.ProjectDir); err == nil {
		if err := CopyFile(yamlPath, path.Join(options.ReportResultsPath(), "qodana.yaml")); err != nil {
			log.Errorf("Error while copying qodana.yaml: %s", err)
			return err
		}
	}
	return nil
}

func convertReportToCloudFormat(options *QodanaOptions, mountInfo *MountInfo) error {
	log.Debugf("Generating report to %s...", options.ReportResultsPath())
	args := converterArgs(options, mountInfo)
	stdout, _, res, err := LaunchAndLog(options, "converter", args...)
	if res != 0 {
		return fmt.Errorf("converter exited with non-zero status code: %d", res)
	}
	if err != nil {
		return fmt.Errorf("error while running converter: %s", err)
	}
	if strings.Contains(stdout, "java.lang") {
		return fmt.Errorf("exception occured while generating report: %s", stdout)
	}
	return nil
}

func copySarifToReportPath(options *QodanaOptions) error {
	destination := filepath.Join(options.ReportResultsPath(), "qodana.sarif.json")
	if err := CopyFile(options.GetSarifPath(), destination); err != nil {
		return fmt.Errorf("problem while copying the report %e", err)
	}
	if err := MakeShortSarif(destination, options.GetShortSarifPath()); err != nil {
		return fmt.Errorf("problem while making short sarif %e", err)
	}
	return nil
}

func converterArgs(options *QodanaOptions, mountInfo *MountInfo) []string {
	return []string{QuoteForWindows(mountInfo.JavaPath), "-jar", QuoteForWindows(mountInfo.Converter), "-s", QuoteForWindows(options.ProjectDir), "-d", QuoteForWindows(options.ResultsDir), "-o", QuoteForWindows(options.ReportResultsPath()), "-n", "result-allProblems.json", "-f"}
}

func printQodanaLogo(options *QodanaOptions, linterInfo *LinterInfo) {
	fmt.Println("\nLog directory: " + options.LogDirPath())
	fmt.Print(QodanaLogo(linterInfo.LinterName, linterInfo.LinterVersion))
}
