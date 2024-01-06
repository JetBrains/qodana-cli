package platform

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2023/cloud"
	"github.com/JetBrains/qodana-cli/v2023/tooling"
	log "github.com/sirupsen/logrus"
	"os"
	"path"
	"path/filepath"
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
	cloud.SetupLicenseToken(options.LoadToken(false, options.RequiresToken(false)))
	options.LicensePlan, err = cloud.GetLicensePlan()
	if err != nil {
		if !linterInfo.IsEap {
			return fmt.Errorf("failed to get license plan: %w", err)
		}
		println("Qodana license plan: EAP license.")
		options.LicensePlan = "COMMUNITY"
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

func RunAnalysis(options *QodanaOptions) int {
	linterOptions := options.GetLinterSpecificOptions()
	if linterOptions == nil {
		ErrorMessage("linter specific options are not set")
		return 1
	}
	mountInfo := (*linterOptions).GetMountInfo()
	linterInfo := (*linterOptions).GetInfo(options)
	err := setup(options)
	if err != nil {
		ErrorMessage(err.Error())
		return 1
	}
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
		return 1
	}

	log.Debugf("Java executable path: %s", mountInfo.JavaPath)
	analysisResult, err := computeBaselinePrintResults(options, mountInfo)
	if err != nil {
		log.Error(err)
		return 1
	}

	source := filepath.Join(options.ResultsDir, "qodana.sarif.json")
	destination := filepath.Join(options.ReportResultsPath(), "qodana.sarif.json")
	if err := CopyFile(source, destination); err != nil {
		log.Fatal(err)
	}
	if err := MakeShortSarif(destination, options.GetShortSarifPath()); err != nil {
		log.Fatal(err)
	}

	log.Debugf("Generating report to %s...", options.ReportResultsPath())
	res, err := RunCmd("", QuoteForWindows(mountInfo.JavaPath), "-jar", QuoteForWindows(mountInfo.Converter), "-s", QuoteForWindows(options.ProjectDir), "-d", QuoteForWindows(options.ResultsDir), "-o", QuoteForWindows(options.ReportResultsPath()), "-n", "result-allProblems.json", "-f")
	if err != nil {
		log.Errorf("Error while generating report: %s", err)
		return res
	}

	if yamlPath, err := GetQodanaYamlPath(options.ProjectDir); err == nil {
		if err := CopyFile(yamlPath, path.Join(options.ReportResultsPath(), "qodana.yaml")); err != nil {
			log.Errorf("Error while copying qodana.yaml: %s", err)
			return 1
		}
	}

	if cloud.Token.Token != "" {
		fmt.Println("Publishing report ...")
		SendReport(options, cloud.Token.Token, QuoteForWindows(filepath.Join(options.CacheDir, PublisherJarName)), QuoteForWindows(mountInfo.JavaPath))
	} else {
		fmt.Println("License token is not set, skipping report publishing")
	}

	logProjectClose(eventsCh)
	return analysisResult
}

func printQodanaLogo(options *QodanaOptions, linterInfo *LinterInfo) {
	fmt.Println("\nLog directory: " + options.LogDirPath())
	fmt.Print(QodanaLogo(linterInfo.LinterName, linterInfo.LinterVersion))
}
