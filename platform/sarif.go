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
	"encoding/json"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform/msg"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdenv"
	"github.com/JetBrains/qodana-cli/v2024/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/v2024/sarif"
	"github.com/google/uuid"
	bbapi "github.com/reviewdog/go-bitbucket"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// https://www.jetbrains.com/help/qodana/qodana-sarif-output.html
const (
	baselineStateEmpty     = ""          // baselineStateEmpty default baseline state (not set)
	baselineStateNew       = "new"       // baselineStateNew new baseline state
	baselineStateUnchanged = "unchanged" // baselineStateUnchanged unchanged baseline state
	extension              = ".sarif.json"
	qodanaCritical         = "Critical"
	qodanaHigh             = "High"
	qodanaModerate         = "Moderate"
	qodanaLow              = "Low"
	qodanaInfo             = "Info"
	sarifError             = "error"
	sarifWarning           = "warning"
	sarifNote              = "note"
)

func MergeSarifReports(c thirdpartyscan.Context, deviceId string) (int, error) {
	tmpResultsDir := GetTmpResultsDir(c.ResultsDir())
	files, err := findSarifFiles(tmpResultsDir)
	sort.Strings(files)
	if err != nil {
		return 0, fmt.Errorf("Error locating SARIF files: %s\n", err)
	}

	if len(files) == 0 {
		return 0, fmt.Errorf("No SARIF files (file names ending with .sarif.json) found in %s\n", tmpResultsDir)
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
			toReplace := c.ProjectDir()
			if !strings.HasSuffix(toReplace, string(os.PathSeparator)) {
				toReplace += string(os.PathSeparator)
			}
			location.PhysicalLocation.ArtifactLocation.Uri = strings.TrimPrefix(
				location.PhysicalLocation.ArtifactLocation.Uri,
				toReplace,
			)
		}
	}
	finalReport.Runs[0].Results = removeDuplicates(finalReport.Runs[0].Results)

	SetVersionControlParams(c, deviceId, finalReport)

	totalProblems := len(finalReport.Runs[0].Results)

	err = WriteReport(GetSarifPath(c.ResultsDir()), finalReport)
	if err != nil {
		return 0, err
	}
	return totalProblems, nil
}

func removeDuplicates(results []sarif.Result) []sarif.Result {
	if len(results) == 0 {
		return results
	}
	seen := make(map[string]struct{}, len(results))
	writeIndex := 0

	for _, result := range results {
		if result.PartialFingerprints != nil {
			fingerPrint := getFingerprint(&result)
			if fingerPrint != "" {
				if _, exists := seen[fingerPrint]; exists {
					continue
				}
				seen[fingerPrint] = struct{}{}
			}
		}
		results[writeIndex] = result
		writeIndex++
	}

	if len(results) != writeIndex {
		log.Warnf("Removed duplicates: %d", len(results)-writeIndex)
	}

	return results[:writeIndex]
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
		return fmt.Errorf("error reading SARIF %s: no runs found", sarifPath)
	}
	report.Runs[0].Tool.Extensions = []sarif.ToolComponent{}
	report.Runs[0].Tool.Driver.Taxa = []sarif.ReportingDescriptor{}
	report.Runs[0].Tool.Driver.Rules = []sarif.ReportingDescriptor{}
	report.Runs[0].Results = []sarif.Result{}
	report.Runs[0].Artifacts = []sarif.Artifact{}
	return WriteReport(shortSarifPath, report)
}

func SetVersionControlParams(c thirdpartyscan.Context, deviceId string, finalReport *sarif.Report) {
	linterInfo := c.LinterInfo()
	vcd, err := GetVersionDetails(c.ProjectDir())
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
	err := filepath.Walk(
		root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), extension) {
				files = append(files, path)
			}
			return nil
		},
	)
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

func ReadReportFromString(sarifStr string) (*sarif.Report, error) {
	var r sarif.Report
	if err := json.Unmarshal([]byte(sarifStr), &r); err != nil {
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

func getRuleDescription(report *sarif.Report, ruleId string) string {
	for _, run := range report.Runs {
		for _, extension := range run.Tool.Extensions {
			for _, rule := range extension.Rules {
				if rule.Id == ruleId {
					return rule.ShortDescription.Text
				}
			}
		}
	}
	return ""
}

// ProcessSarif concludes the result of analysis based on provided SARIF file
// - can print problems to the output
// - can create GitLab CodeQuality issues report
// - can submit problems to BitBucket Code Insights
func ProcessSarif(sarifPath, analysisId, reportUrl string, printProblems, codeClimate, codeInsights bool) {
	newProblems := 0
	s, err := ReadReport(sarifPath)
	if err != nil {
		log.Fatal(err)
	}
	var codeClimateIssues = make([]CCIssue, 0)
	var codeInsightIssues = make([]bbapi.ReportAnnotation, 0)
	rulesDescriptions := make(map[string]string)
	if printProblems {
		msg.EmptyMessage()
	}
	for _, run := range s.Runs {
		for _, r := range run.Results {
			ruleId := r.RuleId
			message := r.Message.Text
			baselineState := baselineStateEmpty
			if r.BaselineState != nil {
				baselineState = r.BaselineState.(string)
			}
			if baselineState == baselineStateNew || baselineState == baselineStateEmpty {
				newProblems++
			}
			if len(r.Locations) > 0 && baselineState != baselineStateUnchanged {
				if codeClimate {
					codeClimateIssues = append(codeClimateIssues, sarifResultToCodeClimate(&r))
				}
				if codeInsights {
					ruleDescription, ok := rulesDescriptions[ruleId]
					if !ok {
						ruleDescription = getRuleDescription(s, ruleId)
						rulesDescriptions[ruleId] = ruleDescription
					}
					codeInsightIssues = append(codeInsightIssues, buildAnnotation(&r, ruleDescription, reportUrl))
				}
				if printProblems {
					printSarifProblem(&r, ruleId, message)
				}
			}
		}
	}
	if codeClimate {
		err = writeGlCodeQualityReport(codeClimateIssues, sarifPath)
		if err != nil {
			log.Warnf("Problems writing GitLab CodeQuality report: %v", err)
		}
	}
	if codeInsights {
		err = sendBitBucketReport(codeInsightIssues, s.Runs[0].Tool.Driver.FullName, reportUrl, "qodana-"+analysisId)
		if err != nil {
			log.Warnf("Problems sending BitBucket Code Insights report: %v", err)
		}
	}
	if !qdenv.IsContainer() {
		if newProblems == 0 {
			msg.SuccessMessage(msg.GetProblemsFoundMessage(0))
		} else {
			msg.ErrorMessage(msg.GetProblemsFoundMessage(newProblems))
		}
	}
}

func printSarifProblem(r *sarif.Result, ruleId, message string) {
	if r.Locations[0].PhysicalLocation != nil {
		msg.PrintProblem(
			ruleId,
			getSeverity(r),
			message,
			r.Locations[0].PhysicalLocation.ArtifactLocation.Uri,
			int(r.Locations[0].PhysicalLocation.Region.StartLine),
			int(r.Locations[0].PhysicalLocation.Region.StartColumn),
			int(r.Locations[0].PhysicalLocation.ContextRegion.StartLine),
			r.Locations[0].PhysicalLocation.ContextRegion.Snippet.Text,
		)
	} else {
		msg.PrintProblem(
			ruleId,
			getSeverity(r),
			message,
			"",
			0,
			0,
			0,
			"",
		)
	}
}

// getFingerprint returns the fingerprint of the Qodana (or not) SARIF result.
func getFingerprint(r *sarif.Result) string {
	if r != nil && r.PartialFingerprints != nil {
		fingerprint, ok := r.PartialFingerprints["equalIndicator/v2"]
		if ok {
			return fingerprint
		} else {
			fingerprint, ok = r.PartialFingerprints["equalIndicator/v1"]
			if ok {
				return fingerprint
			}
		}
	}
	log.Fatalf("failed to get fingerprint from result: %v", r)
	return ""
}

// getSeverity returns the severity of the Qodana (or not) SARIF result.
func getSeverity(r *sarif.Result) string {
	if r.Properties != nil && r.Properties.AdditionalProperties != nil {
		severity, ok := r.Properties.AdditionalProperties["qodanaSeverity"].(string)
		if ok {
			return severity
		}
	}
	if r.Level != nil {
		log.Debug("failed to get severity from properties, using sarif level")
		return r.Level.(string)
	}
	return sarifNote
}
