package linter

import (
	"encoding/json"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/JetBrains/qodana-cli/v2024/sarif"
	log "github.com/sirupsen/logrus"
	"os"
	"strings"
)

func (o *CltOptions) Setup(_ *platform.QodanaOptions) error {
	return nil
}

func (o *CltOptions) RunAnalysis(opts *platform.QodanaOptions) error {
	options := &LocalOptions{opts}
	yaml := platform.GetQodanaYaml(options.ProjectDir)
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
	finalReport, err := platform.ReadReport(options.GetSarifPath())
	if err != nil {
		return fmt.Errorf("failed to read report: %w", err)
	}
	for _, run := range finalReport.Runs {
		run.Tool.Driver.Taxa = make([]sarif.ReportingDescriptor, 0)
		rules := make([]sarif.ReportingDescriptor, 0)
		for _, rule := range run.Tool.Driver.Rules {
			rule.FullDescription = rule.ShortDescription
			if len(rule.Relationships) == 0 && !strings.HasSuffix(rule.Id, "Errors") {
				rule.Name = rule.ShortDescription.Text
				run.Tool.Driver.Taxa = append(run.Tool.Driver.Taxa, rule)
			} else {
				rule.DefaultConfiguration = &sarif.ReportingConfiguration{
					Enabled: true,
				}
				rules = append(rules, rule)
			}
		}
		run.Tool.Driver.Rules = rules
	}

	vcd, err := platform.GetVersionDetails(options.ProjectDir)
	if err != nil {
		log.Errorf("Error getting version control details: %s. Project is probably outside of the Git VCS.", err)
	} else {
		finalReport.Runs[0].VersionControlProvenance = make([]sarif.VersionControlDetails, 0)
		finalReport.Runs[0].VersionControlProvenance = append(finalReport.Runs[0].VersionControlProvenance, vcd)
	}

	deviceId := platform.GetDeviceIdSalt()[0]
	if deviceId != "" {
		finalReport.Runs[0].Properties = &sarif.PropertyBag{}
		finalReport.Runs[0].Properties.AdditionalProperties = map[string]interface{}{
			"deviceId": deviceId,
		}
	}

	if options.GetLinterInfo().ProductCode != "" {
		finalReport.Runs[0].Tool.Driver.Name = options.GetLinterInfo().ProductCode
	}
	if options.GetLinterInfo().LinterName != "" {
		finalReport.Runs[0].Tool.Driver.FullName = options.GetLinterInfo().LinterName
	}

	finalReport.Runs[0].AutomationDetails = &sarif.RunAutomationDetails{
		Guid: platform.RunGUID(),
		Id:   platform.ReportId(options.GetLinterInfo().ProductCode),
		Properties: &sarif.PropertyBag{
			AdditionalProperties: map[string]interface{}{
				"jobUrl": platform.JobUrl(),
			},
		},
	}

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
