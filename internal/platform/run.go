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
	"os"
	"path"
	"path/filepath"

	"github.com/JetBrains/qodana-cli/internal/cloud"
	platformcmd "github.com/JetBrains/qodana-cli/internal/platform/cmd"
	"github.com/JetBrains/qodana-cli/internal/platform/commoncontext"
	"github.com/JetBrains/qodana-cli/internal/platform/effectiveconfig"
	"github.com/JetBrains/qodana-cli/internal/platform/msg"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/JetBrains/qodana-cli/internal/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/internal/platform/tokenloader"
	"github.com/JetBrains/qodana-cli/internal/platform/utils"
	log "github.com/sirupsen/logrus"
)

func RunThirdPartyLinterAnalysis(
	cliOptions platformcmd.CliOptions,
	linter ThirdPartyLinter,
	linterInfo thirdpartyscan.LinterInfo,
) (int, error) {
	qdenv.InitializeQodanaGlobalEnv(cliOptions)

	var err error

	commonCtx := commoncontext.Compute3rdParty(
		linterInfo.LinterName,
		linterInfo.IsEap,
		cliOptions.CacheDir,
		cliOptions.ResultsDir,
		cliOptions.ReportDir,
		qdenv.GetQodanaGlobalEnv(qdenv.QodanaToken),
		cliOptions.ClearCache,
		cliOptions.ProjectDir,
		cliOptions.RepositoryRoot,
	)
	commonCtx, err = correctInitArgsForThirdParty(commonCtx)
	if err != nil {
		msg.ErrorMessage(err.Error())
		return 1, err
	}
	resultDir := commonCtx.ResultsDir
	defer changeResultDirPermissionsInContainer(resultDir)

	thirdPartyCloudData := checkLinterLicense(commonCtx)

	printLinterLicense(thirdPartyCloudData.LicensePlan, linterInfo)
	printQodanaLogo(commonCtx.LogDir(), commonCtx.CacheDir, linterInfo)

	mountInfo := extractUtils(linter, commonCtx.CacheDir)

	localQodanaYamlFullPath := qdyaml.GetLocalNotEffectiveQodanaYamlFullPath(
		commonCtx.ProjectDir,
		cliOptions.ConfigName,
	)

	effectiveDir, cleanup, err := utils.CreateTempDir("qodana-effective-config")
	if err != nil {
		return 1, fmt.Errorf("failed to create qodana effective configuration dir %v", err)
	}
	defer cleanup()

	qodanaConfigEffectiveFiles, err := effectiveconfig.CreateEffectiveConfigFiles(
		commonCtx.CacheDir,
		localQodanaYamlFullPath,
		cliOptions.GlobalConfigurationsDir,
		cliOptions.GlobalConfigurationId,
		effectiveDir,
		commonCtx.LogDir(),
	)
	if err != nil {
		log.Fatalf("Failed to load Qodana configuration %s", err)
	}
	qodanaYamlConfig := thirdpartyscan.QodanaYamlConfig{}
	if qodanaConfigEffectiveFiles.EffectiveQodanaYamlPath != "" {
		yaml := qdyaml.LoadQodanaYamlByFullPath(qodanaConfigEffectiveFiles.EffectiveQodanaYamlPath)
		qodanaYamlConfig = thirdpartyscan.YamlConfig(yaml)
	}

	context := thirdpartyscan.ComputeContext(
		cliOptions,
		commonCtx,
		linterInfo,
		mountInfo,
		thirdPartyCloudData,
		qodanaYamlConfig,
	)

	LogContext(&context)

	events := make([]FuserEvent, 0)
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
		return 1, err
	}

	thresholds := getFailureThresholds(context)
	var analysisResult int
	if analysisResult, err = computeBaselinePrintResults(context, thresholds); err != nil {
		msg.ErrorMessage(err.Error())
		return 1, err
	}
	if qodanaConfigEffectiveFiles.EffectiveQodanaYamlPath != "" {
		err = copyQodanaYamlToLogDir(qodanaConfigEffectiveFiles.EffectiveQodanaYamlPath, context.LogDir())
		if err != nil {
			msg.ErrorMessage(err.Error())
			return 1, err
		}
	}
	if context.SaveReport() || context.ShowReport() {
		commoncontext.SaveReport(context.ResultsDir(), context.ReportDir(), context.CacheDir())
	}
	sendReportToQodanaServer(context)
	newReportUrl := cloud.GetReportUrl(context.ResultsDir())
	ProcessSarif(
		filepath.Join(context.ResultsDir(), commoncontext.QodanaSarifName),
		context.AnalysisId(),
		newReportUrl,
		false,
		context.GenerateCodeClimateReport(),
		context.SendBitBucketInsights(),
	)
	err = writeShortSarifReport(context)
	if err != nil {
		log.Warnf("Problems writing short SARIF report: %v", err)
	}

	commoncontext.InteractiveShowReport(
		context.ShowReport(),
		context.CacheDir(),
		context.ResultsDir(),
		context.ReportDir(),
		context.ShowReportPort(),
	)
	return analysisResult, nil
}

func correctInitArgsForThirdParty(commonCtx commoncontext.Context) (commoncontext.Context, error) {
	empty := commoncontext.Context{}
	var err error

	if commonCtx.ResultsDir, err = filepath.Abs(commonCtx.ResultsDir); err != nil {
		return empty, fmt.Errorf("failed to get absolute path to results directory: %w", err)
	}

	if commonCtx.ReportDir, err = filepath.Abs(commonCtx.ReportDir); err != nil {
		return empty, fmt.Errorf("failed to get absolute path to report directory: %w", err)
	}

	tmpResultsDir := GetTmpResultsDir(commonCtx.ResultsDir)
	if _, err := os.Stat(tmpResultsDir); err == nil {
		if err := os.RemoveAll(tmpResultsDir); err != nil {
			return empty, fmt.Errorf("failed to remove folder with temporary data: %w", err)
		}
	}

	directories := []string{
		commonCtx.ResultsDir,
		commonCtx.LogDir(),
		commonCtx.CacheDir,
		ReportResultsPath(commonCtx.ReportDir),
		tmpResultsDir,
	}
	for _, dir := range directories {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return empty, fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		}
	}
	return commonCtx, nil
}

func checkLinterLicense(loader tokenloader.CloudTokenLoader) thirdpartyscan.ThirdPartyStartupCloudData {
	licensePlan := cloud.CommunityLicensePlan
	token := tokenloader.LoadCloudUploadToken(loader, false, false, true)
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
			LogDir:     c.LogDir(),
			AnalysisId: c.AnalysisId(),
		}

		SendReport(
			c.CacheDir(),
			publisher,
			cloud.Token.Token,
		)
	} else {
		fmt.Println("Skipping report publishing")
	}
}

func copyQodanaYamlToLogDir(qodanaYamlFullPath string, logDir string) error {
	if _, err := os.Stat(qodanaYamlFullPath); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := utils.CopyFile(qodanaYamlFullPath, path.Join(logDir, "qodana.yaml")); err != nil {
		log.Errorf("Error while copying qodana.yaml: %s", err)
		return err
	}
	return nil
}

func printQodanaLogo(logDir string, cacheDir string, linterInfo thirdpartyscan.LinterInfo) {
	fmt.Println("\nLog directory: " + logDir)
	fmt.Println("Cache directory: " + cacheDir)
	fmt.Print(qodanaLogo(linterInfo.LinterPresentableName, linterInfo.LinterVersion, linterInfo.IsEap))
}

// qodanaLogo prepares the info message for the tool
func qodanaLogo(toolDesc string, version string, eap bool) string {
	eapString := ""
	if eap {
		eapString = "EAP"
	}
	return fmt.Sprintf(
		`
          _              _
         /\ \           /\ \        %s %s %s
        /  \ \         /  \ \       Documentation
       / /\ \ \       / /\ \ \      https://jb.gg/qodana-docs
      / / /\ \ \     / / /\ \ \     Contact us at
     / / /  \ \_\   / / /  \ \_\    qodana-support@jetbrains.com
    / / / _ / / /  / / /   / / /    Or via our issue tracker
   / / / /\ \/ /  / / /   / / /     https://jb.gg/qodana-issue
  / / /__\ \ \/  / / /___/ / /      Or share your feedback at our forum
 / / /____\ \ \ / / /____\/ /       https://jb.gg/qodana-forum
 \/________\_\/ \/_________/

`, toolDesc, version, eapString,
	)
}

func changeResultDirPermissionsInContainer(resultDir string) {
	if !qdenv.IsContainer() {
		return
	}
	err := ChangeResultsPermissionsRecursively(resultDir)
	if err != nil {
		msg.ErrorMessage("Unable to change permissions in %s: %s", resultDir, err)
	}
}

// TODO: think about removing short sarif generation from ultimate
func writeShortSarifReport(context thirdpartyscan.Context) error {
	outputPath := GetShortSarifPath(context.ResultsDir())
	sarifPath := GetSarifPath(context.ResultsDir())
	log.Debugf("Creating short SARIF report at %s from %s...", outputPath, sarifPath)
	return MakeShortSarif(sarifPath, outputPath)
}
