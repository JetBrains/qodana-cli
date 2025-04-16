package main

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2025/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
	"strings"
)

// Find qodana.yaml, run bootstrap, find enabled checks and return them formatted as an argument for clang-tidy.
func processConfig(c thirdpartyscan.Context) (string, error) {
	var excludeRules []string
	var includeRules []string

	yaml := c.QodanaYamlConfig()
	var checks = ""
	utils.Bootstrap(yaml.Bootstrap, c.ProjectDir())
	if yaml.Version != "" || len(yaml.Includes) > 0 || len(yaml.Excludes) > 0 {
		fmt.Println("Found qodana.yaml. Note that only bootstrap command and inspection names from include and exclude sections are supported.")
		for _, include := range yaml.Includes {
			if strings.HasPrefix(strings.TrimSpace(include.Name), "clion-") {
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
