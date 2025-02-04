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

func RunThirdPartyLinterAnalysis(options *QodanaOptions) (int, error) {
	linterOptions, mountInfo, linterInfo, err := getLinterDescriptors(options)
	if err != nil {
		ErrorMessage(err.Error())
		return 1, err
	}
	checkLinterLicense(options)
	printLinterLicense(options, linterInfo)
	printQodanaLogo(options, linterInfo)

	defineResultAndCacheDir(options)
	if err = ensureWorkingDirsCreated(options, mountInfo); err != nil {
		ErrorMessage(err.Error())
		return 1, err
	}

	yaml := getQodanaYaml(options)
	if err = (*linterOptions).Setup(options); err != nil {
		return 1, fmt.Errorf("failed to run linter specific setup procedures: %w", err)
	}
	options.LogOptions()

	defer cleanupUtils()
	extractUtils(options)

	events := make([]tooling.FuserEvent, 0)
	eventsCh := createFuserEventChannel(&events)
	defer func() {
		logProjectClose(eventsCh, options, linterInfo)
		sendFuserEvents(eventsCh, &events, options, GetDeviceIdSalt()[0])
	}()
	logOs(eventsCh, options, linterInfo)
	logProjectOpen(eventsCh, options, linterInfo)

	if err = (*linterOptions).RunAnalysis(options, yaml); err != nil {
		ErrorMessage(err.Error())
		return 1, err
	}
	log.Debugf("Java executable path: %s", mountInfo.JavaPath)

	thresholds := getFailureThresholds(yaml, options)
	var analysisResult int
	if analysisResult, err = computeBaselinePrintResults(options, mountInfo, thresholds); err != nil {
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
	return analysisResult, nil
}

func getQodanaYaml(options *QodanaOptions) *QodanaYaml {
	qodanaYamlPath := FindQodanaYaml(options.ProjectDir)
	if options.ConfigName != "" {
		qodanaYamlPath = options.ConfigName
	}
	return LoadQodanaYaml(options.ProjectDir, qodanaYamlPath)
}

func ensureWorkingDirsCreated(options *QodanaOptions, mountInfo *MountInfo) error {
	var err error

	if mountInfo.JavaPath, err = getJavaExecutablePath(); err != nil {
		return fmt.Errorf("failed to get java executable path: %w", err)
	}

	if options.ResultsDir, err = filepath.Abs(options.ResultsDir); err != nil {
		return fmt.Errorf("failed to get absolute path to results directory: %w", err)
	}

	if options.ReportDir, err = filepath.Abs(options.reportDirPath()); err != nil {
		return fmt.Errorf("failed to get absolute path to report directory: %w", err)
	}

	if _, err := os.Stat(options.GetTmpResultsDir()); err == nil {
		if err := os.RemoveAll(options.GetTmpResultsDir()); err != nil {
			return fmt.Errorf("failed to remove folder with temporary data: %w", err)
		}
	}

	directories := []string{
		options.ResultsDir,
		options.LogDirPath(),
		options.CacheDir,
		options.ReportResultsPath(),
		options.GetTmpResultsDir(),
	}
	for _, dir := range directories {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		}
	}
	return nil
}

func checkLinterLicense(options *QodanaOptions) {
	options.LicensePlan = cloud.CommunityLicensePlan
	token := options.LoadToken(false, false, true)
	if token != "" {
		options.Setenv(QodanaToken, token)
	}
	cloud.SetupLicenseToken(token)
	if cloud.Token.Token != "" {
		licenseData := cloud.GetCloudApiEndpoints().GetLicenseData(cloud.Token.Token)
		token.ValidateTokenPrintProject(cloud.Token.Token)
		options.LicensePlan = licenseData.LicensePlan
		options.ProjectIdHash = licenseData.ProjectIdHash
	}
}

func printLinterLicense(options *QodanaOptions, linterInfo *LinterInfo) {
	licenseString := options.LicensePlan
	if cloud.Token.Token == "" && linterInfo.IsEap {
		licenseString = "EAP license"
	}
	SuccessMessage("Qodana license plan: %s", licenseString)
}

func getLinterDescriptors(options *QodanaOptions) (*ThirdPartyOptions, *MountInfo, *LinterInfo, error) {
	linterOptions := options.GetLinterSpecificOptions()
	if linterOptions == nil {
		return nil, nil, nil, fmt.Errorf("linter specific options are not set")
	}
	mountInfo := (*linterOptions).GetMountInfo()
	linterInfo := (*linterOptions).GetInfo(options)
	return linterOptions, mountInfo, linterInfo, nil
}

func defineResultAndCacheDir(options *QodanaOptions) {
	// we don't provide default for cache dir, since we don't want to compute options.id without knowing the exact folder
	if options.CacheDir == "" {
		options.CacheDir = options.GetCacheDir()
	}
	if options.ResultsDir == "" {
		options.ResultsDir = options.resultsDirPath()
	}
}

func sendReportToQodanaServer(options *QodanaOptions, mountInfo *MountInfo) {
	if cloud.Token.IsAllowedToSendReports() {
		fmt.Println("Publishing report ...")
		SendReport(options, cloud.Token.Token, QuoteForWindows(filepath.Join(options.CacheDir, PublisherJarName)), QuoteForWindows(mountInfo.JavaPath))
	} else {
		fmt.Println("Skipping report publishing")
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
	fmt.Println("Cache directory: " + options.GetCacheDir())
	fmt.Print(QodanaLogo(linterInfo.LinterName, linterInfo.LinterVersion, linterInfo.IsEap))
}
