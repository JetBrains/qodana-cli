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

package cdnet

import (
	"encoding/json"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/JetBrains/qodana-cli/v2024/sarif"
	"os"
	"path/filepath"
)

func (o *CltOptions) Setup(_ *platform.QodanaOptions) error {
	return nil
}

func (o *CltOptions) RunAnalysis(opts *platform.QodanaOptions) error {
	options := &LocalOptions{opts}
	yaml := platform.GetQodanaYamlOrDefault(options.ProjectDir)
	platform.Bootstrap(yaml.Bootstrap, options.ProjectDir)
	args, err := o.computeCdnetArgs(opts, options, yaml)
	if err != nil {
		return err
	}
	if platform.IsNugetConfigNeeded() {
		platform.PrepareNugetConfig(os.Getenv("HOME"))
	}
	platform.UnsetNugetVariables()
	ret, err := platform.RunCmd(
		platform.QuoteForWindows(options.ProjectDir),
		args...,
	)
	if err != nil {
		return err
	}
	if ret != 0 {
		return fmt.Errorf("analysis exited with code: %d", ret)
	}
	err = patchReport(options)
	return err
}

func patchReport(options *LocalOptions) error {
	if err := copyOriginalReportToLog(options); err != nil {
		return err
	}
	finalReport, err := platform.ReadReport(options.GetSarifPath())
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

	platform.SetVersionControlParams(options.QodanaOptions, platform.GetDeviceIdSalt()[0], finalReport)

	// serialize object skipping empty fields
	fatBytes, err := json.MarshalIndent(finalReport, "", " ")
	if err != nil {
		return fmt.Errorf("error marshalling report: %w", err)
	}

	f, err := os.Create(options.GetSarifPath())
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

func copyOriginalReportToLog(options *LocalOptions) error {
	destination := filepath.Join(options.LogDirPath(), "clt.original.sarif.json")
	if err := platform.CopyFile(options.GetSarifPath(), destination); err != nil {
		return fmt.Errorf("problem while copying the original CLT report %e", err)
	}
	return nil
}
