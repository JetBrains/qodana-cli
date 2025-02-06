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

package main

import (
	"encoding/json"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/JetBrains/qodana-cli/v2024/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/v2024/sarif"
	"os"
	"path/filepath"
)

type CdnetLinter struct {
}

func (l CdnetLinter) ComputeNewLinterInfo(linterInfo platform.LinterInfo, _ bool) (platform.LinterInfo, error) {
	return linterInfo, nil
}

func (l CdnetLinter) RunAnalysis(c thirdpartyscan.Context) error {
	platform.Bootstrap(c.QodanaYaml().Bootstrap, c.ProjectDir())
	args, err := l.computeCdnetArgs(c)
	if err != nil {
		return err
	}
	if platform.IsNugetConfigNeeded() {
		platform.PrepareNugetConfig(os.Getenv("HOME"))
	}
	platform.UnsetNugetVariables()
	ret, err := platform.RunCmd(
		platform.QuoteForWindows(c.ProjectDir()),
		args...,
	)
	if err != nil {
		return err
	}
	if ret != 0 {
		return fmt.Errorf("analysis exited with code: %d", ret)
	}
	err = patchReport(c)
	return err
}

func patchReport(c thirdpartyscan.Context) error {
	sarifPath := platform.GetSarifPath(c.ResultsDir())
	if err := copyOriginalReportToLog(c.LogDir(), sarifPath); err != nil {
		return err
	}
	finalReport, err := platform.ReadReport(sarifPath)
	if err != nil {
		return fmt.Errorf("failed to read report: %w", err)
	}
	for _, run := range finalReport.Runs {
		rules := make([]sarif.ReportingDescriptor, 0)
		for _, rule := range run.Tool.Driver.Rules {
			if rule.FullDescription == nil {
				rule.FullDescription = rule.ShortDescription
			}
			if rule.ShortDescription == nil {
				rule.ShortDescription = rule.FullDescription
			}

			rule.DefaultConfiguration = &sarif.ReportingConfiguration{
				Enabled: true,
			}
			rules = append(rules, rule)
		}
		run.Tool.Driver.Rules = rules

		taxonomy := make([]sarif.ReportingDescriptor, 0)
		for _, taxa := range run.Tool.Driver.Taxa {
			if taxa.Name == "" {
				taxa.Name = taxa.Id
			}
			taxonomy = append(taxonomy, taxa)
		}
		run.Tool.Driver.Taxa = taxonomy
	}

	platform.SetVersionControlParams(c, platform.GetDeviceIdSalt()[0], finalReport)

	// serialize object skipping empty fields
	fatBytes, err := json.MarshalIndent(finalReport, "", " ")
	if err != nil {
		return fmt.Errorf("error marshalling report: %w", err)
	}

	f, err := os.Create(sarifPath)
	if err != nil {
		return fmt.Errorf("error creating resulting SARIF file: %w", err)
	}

	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			fmt.Printf("Error closing resulting SARIF file: %s\n", err)
		}
	}(f)

	_, err = f.Write(fatBytes)
	if err != nil {
		return fmt.Errorf("error writing resulting SARIF file: %w", err)
	}
	return nil
}

func copyOriginalReportToLog(logDir string, sarifPath string) error {
	destination := filepath.Join(logDir, "clt.original.sarif.json")
	if err := platform.CopyFile(sarifPath, destination); err != nil {
		return fmt.Errorf("problem while copying the original CLT report %e", err)
	}
	return nil
}
