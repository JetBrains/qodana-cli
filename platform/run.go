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
	"errors"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/cloud"
	"github.com/JetBrains/qodana-cli/v2024/platform/cmd"
	"github.com/JetBrains/qodana-cli/v2024/platform/msg"
	"github.com/JetBrains/qodana-cli/v2024/platform/platforminit"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdenv"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/v2024/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/v2024/platform/tokenloader"
	"github.com/JetBrains/qodana-cli/v2024/platform/utils"
	"github.com/JetBrains/qodana-cli/v2024/tooling"
	log "github.com/sirupsen/logrus"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func RunThirdPartyLinterAnalysis(
	cliOptions platformcmd.CliOptions,
	linter ThirdPartyLinter,
	linterInfo thirdpartyscan.LinterInfo,
) (thirdpartyscan.Context, int, error) {
	var err error

	initArgs := platforminit.ComputeArgs(
		cliOptions.Linter,
		cliOptions.Ide,
		cliOptions.CacheDir,
		cliOptions.ResultsDir,
		cliOptions.ReportDir,
		GetEnvWithOsEnv(cliOptions, qdenv.QodanaToken),
		GetEnvWithOsEnv(cliOptions, qdenv.QodanaLicenseOnlyToken),
		cliOptions.ClearCache,
		cliOptions.ProjectDir,
		cliOptions.ConfigName,
	)
	initArgs, err = correctInitArgsForThirdParty(initArgs)
	if err != nil {
		msg.ErrorMessage(err.Error())
		return thirdpartyscan.Context{}, 1, err
	}

	thirdPartyCloudData := checkLinterLicense(initArgs)
	isCommunity := thirdPartyCloudData.LicensePlan == cloud.CommunityLicensePlan

	printLinterLicense(thirdPartyCloudData.LicensePlan, linterInfo)
	printQodanaLogo(initArgs.LogDir(), initArgs.CacheDir, linterInfo)

	if linterInfo, err = linter.ComputeNewLinterInfo(linterInfo, isCommunity); err != nil {
		return thirdpartyscan.Context{}, 1, fmt.Errorf("failed to run linter specific setup procedures: %w", err)
	}

	tempMountPath, mountInfo := extractUtils(linter, initArgs.CacheDir, isCommunity)
	defer cleanupUtils(tempMountPath)

	qodanaYamlPath := qdyaml.GetQodanaYamlPathWithProject(initArgs.ProjectDir, cliOptions.ConfigName)
	yaml := qdyaml.LoadQodanaYamlByFullPath(qodanaYamlPath)

	context := thirdpartyscan.ComputeContext(cliOptions, initArgs, linterInfo, mountInfo, thirdPartyCloudData, yaml)

	LogContext(&context)

	events := make([]tooling.FuserEvent, 0)
	eventsCh := createFuserEventChannel(&events)

	projectIdHash := thirdPartyCloudData.ProjectIdHash
	defer func() {
		logProjectClose(eventsCh, linterInfo, projectIdHash)
		sendFuserEvents(eventsCh, &events, context, GetDeviceIdSalt()[0])
	}()
	logOs(eventsCh, linterInfo, projectIdHash)
	logProjectOpen(eventsCh, linterInfo, projectIdHash)

	if err = linter.RunAnalysis(context); err != nil {
		msg.ErrorMessage(err.Error())
		return context, 1, err
	}
	log.Debugf("Java executable path: %s", mountInfo.JavaPath)

	thresholds := getFailureThresholds(context)
	var analysisResult int
	if analysisResult, err = computeBaselinePrintResults(context, thresholds); err != nil {
		msg.ErrorMessage(err.Error())
		return context, 1, err
	}
	if err = copySarifToReportPath(context.ResultsDir()); err != nil {
		msg.ErrorMessage(err.Error())
		return context, 1, err
	}
	if err = convertReportToCloudFormat(context); err != nil {
		msg.ErrorMessage(err.Error())
		return context, 1, err
	}
	resultsPath := ReportResultsPath(context.ResultsDir())
	if err = copyQodanaYamlToReportPath(qodanaYamlPath, resultsPath); err != nil {
		msg.ErrorMessage(err.Error())
		return context, 1, err
	}
	sendReportToQodanaServer(context)
	return context, analysisResult, nil
}

func correctInitArgsForThirdParty(args platforminit.Args) (platforminit.Args, error) {
	empty := platforminit.Args{}
	var err error

	if args.ResultsDir, err = filepath.Abs(args.ResultsDir); err != nil {
		return empty, fmt.Errorf("failed to get absolute path to results directory: %w", err)
	}

	if args.ReportDir, err = filepath.Abs(args.ReportDir); err != nil {
		return empty, fmt.Errorf("failed to get absolute path to report directory: %w", err)
	}

	tmpResultsDir := GetTmpResultsDir(args.ResultsDir)
	if _, err := os.Stat(tmpResultsDir); err == nil {
		if err := os.RemoveAll(tmpResultsDir); err != nil {
			return empty, fmt.Errorf("failed to remove folder with temporary data: %w", err)
		}
	}

	directories := []string{
		args.ResultsDir,
		args.LogDir(),
		args.CacheDir,
		ReportResultsPath(args.ReportDir),
		tmpResultsDir,
	}
	for _, dir := range directories {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return empty, fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		}
	}
	return args, nil
}

func checkLinterLicense(loader tokenloader.CloudTokenLoader) thirdpartyscan.ThirdPartyStartupCloudData {
	licensePlan := cloud.CommunityLicensePlan
	token := tokenloader.LoadCloudToken(loader, false, false, true)
	projectIdHash := ""
	cloud.SetupLicenseToken(token)
	if cloud.Token.Token != "" {
		licenseData := cloud.GetCloudApiEndpoints().GetLicenseData(cloud.Token.Token)
		tokenloader.ValidateTokenPrintProject(cloud.Token.Token)
		licensePlan = licenseData.LicensePlan
		projectIdHash = licenseData.ProjectIdHash
	}
	return thirdpartyscan.ThirdPartyStartupCloudData{
		LicensePlan:   licensePlan,
		QodanaToken:   token,
		ProjectIdHash: projectIdHash,
	}
}

func printLinterLicense(licensePlan string, linterInfo thirdpartyscan.LinterInfo) {
	licenseString := licensePlan
	if cloud.Token.Token == "" && linterInfo.IsEap {
		licenseString = "EAP license"
	}
	msg.SuccessMessage("Qodana license plan: %s", licenseString)
}

func sendReportToQodanaServer(c thirdpartyscan.Context) {
	if cloud.Token.IsAllowedToSendReports() {
		fmt.Println("Publishing report ...")
		publisher := Publisher{
			ResultsDir: c.ResultsDir(),
			ProjectDir: c.ProjectDir(),
			LogDir:     c.LogDir(),
			AnalysisId: c.AnalysisId(),
		}
		SendReport(
			publisher,
			cloud.Token.Token,
			utils.QuoteForWindows(filepath.Join(c.CacheDir(), PublisherJarName)),
			utils.QuoteForWindows(c.MountInfo().JavaPath),
		)
	} else {
		fmt.Println("Skipping report publishing")
	}
}

func copyQodanaYamlToReportPath(qodanaYamlFullPath string, reportResultsPath string) error {
	if _, err := os.Stat(qodanaYamlFullPath); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := utils.CopyFile(qodanaYamlFullPath, path.Join(reportResultsPath, "qodana.yaml")); err != nil {
		log.Errorf("Error while copying qodana.yaml: %s", err)
		return err
	}
	return nil
}

func convertReportToCloudFormat(c thirdpartyscan.Context) error {
	reportResultsPath := ReportResultsPath(c.ResultsDir())
	log.Debugf("Generating report to %s...", reportResultsPath)
	args := converterArgs(c)
	stdout, _, res, err := utils.LaunchAndLog(c.LogDir(), "converter", args...)
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

func copySarifToReportPath(resultsDir string) error {
	reportResultsPath := ReportResultsPath(resultsDir)
	sarifPath := GetSarifPath(resultsDir)
	shortSarifPath := GetShortSarifPath(resultsDir)

	destination := filepath.Join(reportResultsPath, "qodana.sarif.json")
	if err := utils.CopyFile(sarifPath, destination); err != nil {
		return fmt.Errorf("problem while copying the report %e", err)
	}
	if err := MakeShortSarif(destination, shortSarifPath); err != nil {
		return fmt.Errorf("problem while making short sarif %e", err)
	}
	return nil
}

func converterArgs(c thirdpartyscan.Context) []string {
	reportResultsPath := ReportResultsPath(c.ResultsDir())
	return []string{
		utils.QuoteForWindows(c.MountInfo().JavaPath),
		"-jar",
		utils.QuoteForWindows(c.MountInfo().Converter),
		"-s",
		utils.QuoteForWindows(c.ProjectDir()),
		"-d",
		utils.QuoteForWindows(c.ResultsDir()),
		"-o",
		utils.QuoteForWindows(reportResultsPath),
		"-n",
		"result-allProblems.json",
		"-f",
	}
}

func printQodanaLogo(logDir string, cacheDir string, linterInfo thirdpartyscan.LinterInfo) {
	fmt.Println("\nLog directory: " + logDir)
	fmt.Println("Cache directory: " + cacheDir)
	fmt.Print(platforminit.QodanaLogo(linterInfo.LinterName, linterInfo.LinterVersion, linterInfo.IsEap))
}
