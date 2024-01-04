package linter

import (
	"encoding/json"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2023/core"
	log "github.com/sirupsen/logrus"
	"os"
	"strings"
)

const (
	qodanaNugetUrl      = "QODANA_NUGET_URL"
	qodanaNugetUser     = "QODANA_NUGET_USER"
	qodanaNugetPassword = "QODANA_NUGET_PASSWORD"
	qodanaNugetName     = "QODANA_NUGET_NAME"
)

func (o *CltOptions) Setup(_ *core.Options) error {
	return nil
}

func (o *CltOptions) RunAnalysis(opts *core.Options) error {
	options := &LocalOptions{opts}
	yaml := core.GetQodanaYaml(options.ProjectDir)
	err := core.Bootstrap(options.ProjectDir, yaml)
	if err != nil {
		return err
	}
	args, err := o.computeCdnetArgs(opts, options, yaml)
	if err != nil {
		return err
	}
	if isNugetConfigNeeded() {
		prepareNugetConfig(os.Getenv("HOME"))
	}
	unsetNugetVariables()
	ret, err := core.RunCmd(
		core.QuoteForWindows(options.ProjectDir),
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
	finalReport, err := core.ReadReport(options.GetSarifPath())
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

	vcd, err := core.GetVersionDetails(options.ProjectDir)
	if err != nil {
		log.Errorf("Error getting version control details: %s. Project is probably outside of the Git VCS.", err)
	} else {
		finalReport.Runs[0].VersionControlProvenance = make([]sarif.VersionControlDetails, 0)
		finalReport.Runs[0].VersionControlProvenance = append(finalReport.Runs[0].VersionControlProvenance, vcd)
	}

	if options.DeviceId != "" {
		finalReport.Runs[0].Properties = &sarif.PropertyBag{}
		finalReport.Runs[0].Properties.AdditionalProperties = map[string]interface{}{
			"deviceId": options.DeviceId,
		}
	}

	if options.ProductCode != "" {
		finalReport.Runs[0].Tool.Driver.Name = options.ProductCode
	}
	if options.LinterName != "" {
		finalReport.Runs[0].Tool.Driver.FullName = options.LinterName
	}

	finalReport.Runs[0].AutomationDetails = &sarif.RunAutomationDetails{
		Guid: core.RunGUID(),
		Id:   core.ReportId(options.ProductCode),
		Properties: &sarif.PropertyBag{
			AdditionalProperties: map[string]interface{}{
				"jobUrl": core.JobUrl(),
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
