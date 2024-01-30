package platform

import (
	"encoding/json"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/sarif"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// let's name it sarifutils

const extension = ".sarif.json"

func MergeSarifReports(options *QodanaOptions, deviceId string) (int, error) {
	files, err := findSarifFiles(options.GetTmpResultsDir())
	if err != nil {
		return 0, fmt.Errorf("Error locating SARIF files: %s\n", err)
	}

	if len(files) == 0 {
		return 0, fmt.Errorf("No SARIF files (file names ending with .sarif.json) found in %s\n", options.GetTmpResultsDir())
	}

	ch := make(chan *sarif.Report)
	go collectReports(files, ch)
	finalReport, err := mergeReports(ch)
	if err != nil {
		return 0, fmt.Errorf("Error merging SARIF files: %s\n", err)
	}

	for _, result := range finalReport.Runs[0].Results {
		// update locations[].physicalLocation.artifactLocation.uri by removing the projectDir prefix
		for _, location := range result.Locations {
			if (location.PhysicalLocation == nil) || (location.PhysicalLocation.ArtifactLocation == nil) {
				continue
			}
			toReplace := options.ProjectDir
			if !strings.HasSuffix(toReplace, string(os.PathSeparator)) {
				toReplace += string(os.PathSeparator)
			}
			location.PhysicalLocation.ArtifactLocation.Uri = strings.TrimPrefix(location.PhysicalLocation.ArtifactLocation.Uri, toReplace)
		}
	}

	SetVersionControlParams(options, deviceId, finalReport)

	totalProblems := len(finalReport.Runs[0].Results)

	err = WriteReport(options.GetSarifPath(), finalReport)
	if err != nil {
		return 0, err
	}
	return totalProblems, nil
}

func WriteReport(path string, finalReport *sarif.Report) error {
	// serialize object skipping empty fields
	fatBytes, err := json.MarshalIndent(finalReport, "", " ")
	if err != nil {
		return fmt.Errorf("Error marshalling report: %s\n", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("Error creating resulting SARIF file: %s\n", err)
	}

	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			fmt.Printf("Error closing resulting SARIF file: %s\n", err)
		}
	}(f)

	_, err = f.Write(fatBytes)
	if err != nil {
		return fmt.Errorf("Error writing resulting SARIF file: %s\n", err)
	}
	return nil
}

func MakeShortSarif(sarifPath string, shortSarifPath string) error {
	report, err := ReadReport(sarifPath)
	if err != nil {
		return err
	}

	if len(report.Runs) == 0 {
		return fmt.Errorf("Error reading SARIF %s: no runs found", sarifPath)
	}
	report.Runs[0].Tool.Extensions = []sarif.ToolComponent{}
	report.Runs[0].Tool.Driver.Taxa = []sarif.ReportingDescriptor{}
	report.Runs[0].Tool.Driver.Rules = []sarif.ReportingDescriptor{}
	report.Runs[0].Results = []sarif.Result{}
	report.Runs[0].Artifacts = []sarif.Artifact{}
	return WriteReport(shortSarifPath, report)
}

func SetVersionControlParams(options *QodanaOptions, deviceId string, finalReport *sarif.Report) {
	linterOptions := options.GetLinterSpecificOptions()
	if linterOptions == nil {
		log.Errorf("Error getting linter-specific options")
		return
	}
	linterInfo := (*linterOptions).GetInfo(options)
	vcd, err := GetVersionDetails(options.ProjectDir)
	if err != nil {
		log.Errorf("Error getting version control details: %s. Project is probably outside of the Git VCS.", err)
	} else {
		finalReport.Runs[0].VersionControlProvenance = make([]sarif.VersionControlDetails, 0)
		finalReport.Runs[0].VersionControlProvenance = append(finalReport.Runs[0].VersionControlProvenance, vcd)
	}

	if deviceId != "" {
		finalReport.Runs[0].Properties = &sarif.PropertyBag{}
		finalReport.Runs[0].Properties.AdditionalProperties = map[string]interface{}{
			"deviceId": deviceId,
		}
	}

	if linterInfo.ProductCode != "" {
		finalReport.Runs[0].Tool.Driver.Name = linterInfo.ProductCode
	}
	if linterInfo.LinterName != "" {
		finalReport.Runs[0].Tool.Driver.FullName = linterInfo.LinterName
	}
	if linterInfo.LinterVersion != "" {
		finalReport.Runs[0].Tool.Driver.Version = linterInfo.LinterVersion
	}

	finalReport.Runs[0].AutomationDetails = &sarif.RunAutomationDetails{
		Guid: RunGUID(),
		Id:   ReportId(linterInfo.ProductCode),
		Properties: &sarif.PropertyBag{
			AdditionalProperties: map[string]interface{}{
				"jobUrl": JobUrl(),
			},
		},
	}
}

func findSarifFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), extension) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func collectReports(files []string, ch chan<- *sarif.Report) {
	for _, file := range files {
		r, err := ReadReport(file)
		if err != nil {
			fmt.Printf("Error reading SARIF %s: %s\n", file, err)
			continue
		}
		ch <- r
	}
	close(ch)
}

func ReadReport(file string) (*sarif.Report, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			fmt.Printf("Error closing SARIF file %s: %s\n", file, err)
		}
	}(f)

	dec := json.NewDecoder(f)
	var r sarif.Report
	if err := dec.Decode(&r); err != nil {
		return nil, err
	}

	return &r, nil
}

func mergeReports(ch <-chan *sarif.Report) (*sarif.Report, error) {
	var finalReport *sarif.Report

	for r := range ch {
		if finalReport == nil {
			// For the first file, keep the toolDesc configuration and initialize the 'Runs' slice
			finalReport = &sarif.Report{
				Schema:  r.Schema,
				Version: r.Version,
				Runs:    make([]sarif.Run, 0, len(r.Runs)),
			}
			finalReport.Runs = append(finalReport.Runs, r.Runs[0])
			finalReport.Runs[0].Results = r.Runs[0].Results
			finalReport.Runs[0].Tool = r.Runs[0].Tool
			continue
		}

		// Append results from each report into the 'Results' slice of the first run of the final report
		for _, run := range r.Runs {
			finalReport.Runs[0].Results = append(finalReport.Runs[0].Results, run.Results...)
			finalReport.Runs[0].Artifacts = append(finalReport.Runs[0].Artifacts, run.Artifacts...)
		}
	}

	return finalReport, nil
}

func RunGUID() string {
	runGUID := os.Getenv("QODANA_AUTOMATION_GUID")
	if runGUID == "" {
		runGUID = uuid.New().String()
	}
	return runGUID
}

func ReportId(projectName string) string {
	reportId := os.Getenv("QODANA_REPORT_ID")
	if reportId != "" {
		return reportId
	}

	projectId := os.Getenv("QODANA_PROJECT_ID")
	if projectId == "" {
		projectId = projectName
	}

	date := time.Now().Format("2006-01-02")
	tool := "qodana"

	return projectId + "/" + tool + "/" + date
}

func JobUrl() string {
	return os.Getenv("QODANA_JOB_URL")
}
