package main

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2025/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
	"strings"
)

var SupportedLicenseTypes = map[string]bool{
	"COMMUNITY":           true,
	"TRIAL_ULTIMATE":      true,
	"TRIAL_ULTIMATE_PLUS": true,
	"ULTIMATE":            true,
	"ULTIMATE_PLUS":       true,
	"PREMIUM":             true,
}

func allowedChecksByLicenseAndYaml(c thirdpartyscan.Context) (string, error) {
	var isCommunity = false
	var excludeRules []string
	var includeRules []string

	if c.IsCommunity() {
		if !c.LinterInfo().IsEap {
			fmt.Println("You are using community version of the linter. Restrictions on the checks apply.")
			excludeRules = append(excludeRules, "clion-*")
			isCommunity = true
		}
	} else if _, exists := SupportedLicenseTypes[c.CloudData().LicensePlan]; !exists {
		return "", fmt.Errorf("unsupported license type: %s", c.CloudData().LicensePlan)
	}

	yaml := c.QodanaYamlConfig()
	var checks = ""
	utils.Bootstrap(yaml.Bootstrap, c.ProjectDir())
	if yaml.Version != "" || len(yaml.Includes) > 0 || len(yaml.Excludes) > 0 {
		fmt.Println("Found qodana.yaml. Note that only bootstrap command and inspection names from include and exclude sections are supported.")
		for _, include := range yaml.Includes {
			if isCommunity && strings.HasPrefix(strings.TrimSpace(include.Name), "clion-") {
				continue
			}
			if strings.ContainsAny(include.Name, "\"") {
				continue
			}
			includeRules = append(includeRules, include.Name)
		}
		for _, exclude := range yaml.Excludes {
			if strings.ContainsAny(exclude.Name, "\"") {
				continue
			}
			excludeRules = append(excludeRules, exclude.Name)
		}
	}
	plusChecks := strings.Join(includeRules, ",")
	for i, minusCheck := range excludeRules {
		excludeRules[i] = "-" + minusCheck
	}
	minusChecks := strings.Join(excludeRules, ",")
	if plusChecks != "" && minusChecks != "" {
		checks = fmt.Sprintf("--checks=%s,%s", plusChecks, minusChecks)
	} else if plusChecks != "" {
		checks = fmt.Sprintf("--checks=%s", plusChecks)
	} else if minusChecks != "" {
		checks = fmt.Sprintf("--checks=%s", minusChecks)
	}
	return checks, nil
}
