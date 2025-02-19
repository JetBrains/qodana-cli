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
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/JetBrains/qodana-cli/v2024/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/v2024/platform/utils"
	"strings"
)

func (l CdnetLinter) computeCdnetArgs(c thirdpartyscan.Context) ([]string, error) {
	target := getSolutionOrProject(c)
	if target == "" {
		return nil, fmt.Errorf("solution/project relative file path is not specified. Use --solution or --project flags or create qodana.yaml file with respective fields")
	}
	var props = ""
	for _, p := range c.Property() {
		if strings.HasPrefix(p, "log.") ||
			strings.HasPrefix(p, "idea.") ||
			strings.HasPrefix(p, "qodana.") ||
			strings.HasPrefix(p, "jetbrains.") {
			continue
		}
		if props != "" {
			props += ";"
		}
		props += p
	}
	dotNet := c.QodanaYamlConfig().DotNet
	if c.CdnetConfiguration() != "" {
		if props != "" {
			props += ";"
		}
		props += "Configuration=" + c.CdnetConfiguration()
	} else if dotNet.Configuration != "" {
		if props != "" {
			props += ";"
		}
		props += "Configuration=" + dotNet.Configuration
	}
	if c.CdnetPlatform() != "" {
		if props != "" {
			props += ";"
		}
		props += "Platform=" + c.CdnetPlatform()
	} else if dotNet.Platform != "" {
		if props != "" {
			props += ";"
		}
		props += "Platform=" + dotNet.Platform
	}
	mountInfo := c.MountInfo()

	sarifPath := platform.GetSarifPath(c.ResultsDir())

	args := []string{
		"dotnet",
		utils.QuoteForWindows(mountInfo.CustomTools[thirdpartyscan.Clt]),
		"inspectcode",
		utils.QuoteForWindows(target),
		"-o=\"" + sarifPath + "\"",
		"-f=\"Qodana\"",
		"--LogFolder=\"" + c.LogDir() + "\"",
	}
	if props != "" {
		args = append(args, "--properties:"+props)
	}
	if c.NoStatistics() {
		args = append(args, "--telemetry-optout")
	}
	if c.CdnetNoBuild() {
		args = append(args, "--no-build")
	}
	return args, nil
}

func getSolutionOrProject(c thirdpartyscan.Context) string {
	var target = ""
	paths := [4]string{
		c.CdnetSolution(),
		c.CdnetProject(),
		c.QodanaYamlConfig().DotNet.Solution,
		c.QodanaYamlConfig().DotNet.Project,
	}
	for _, path := range paths {
		if path != "" {
			target = path
			break
		}
	}
	return target
}
