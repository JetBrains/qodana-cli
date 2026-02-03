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
	"os"
	"path/filepath"

	_ "embed"

	"github.com/JetBrains/qodana-cli/internal/platform"
	"github.com/JetBrains/qodana-cli/internal/platform/nuget"
	"github.com/JetBrains/qodana-cli/internal/platform/strutil"
	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/internal/platform/utils"
	"github.com/JetBrains/qodana-cli/internal/sarif"
)

type CdnetLinter struct {
}

const cltFingeprint = "contextRegionHash/v1"
const qodanaFingeprint = "equalIndicator/v1"
const archive = "clt.zip"
const moniker = "resharper-clt"

func (l CdnetLinter) RunAnalysis(c thirdpartyscan.Context) error {
	utils.Bootstrap(c.QodanaYamlConfig().Bootstrap, c.ProjectDir())
	args, err := l.computeCdnetArgs(c)
	if err != nil {
		return err
	}
	if nuget.IsNugetConfigNeeded() {
		nuget.PrepareNugetConfig(os.Getenv("HOME"))
	}
	nuget.UnsetNugetVariables()
	ret, err := utils.Exec(
		strutil.QuoteForWindows(c.ProjectDir()),
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

//go:generate go run scripts/process-cltzip.go

//go:embed clt.zip
var CltArchive []byte

//go:embed clt.sha256.bin
var CltSha256 []byte

//go:embed clt.path.txt
var CltDllRelativePath string

func (l CdnetLinter) MountTools(path string) (map[string]string, error) {
	val := make(map[string]string)
	val[thirdpartyscan.Clt] = filepath.Join(path, CltDllRelativePath)

	if _, err := os.Stat(val["clt"]); err != nil {
		if os.IsNotExist(err) {
			archivePath := platform.ProcessAuxiliaryTool(archive, moniker, path, CltArchive)
			if err := platform.Decompress(archivePath, path); err != nil {
				return nil, fmt.Errorf("failed to decompress %s archive: %w", moniker, err)
			}
		}
	}
	return val, nil
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

		results := make([]sarif.Result, 0)
		for _, result := range run.Results {
			if result.PartialFingerprints != nil {
				if cltValue := result.PartialFingerprints[cltFingeprint]; cltValue != "" && result.PartialFingerprints[qodanaFingeprint] == "" {
					result.PartialFingerprints[qodanaFingeprint] = cltValue
					delete(result.PartialFingerprints, cltFingeprint)
				}
			}
			results = append(results, result)
		}
		run.Results = results
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
	if err := utils.CopyFile(sarifPath, destination); err != nil {
		return fmt.Errorf("problem while copying the original CLT report: %w", err)
	}
	return nil
}
