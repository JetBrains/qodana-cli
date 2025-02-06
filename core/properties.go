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

package core

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/cloud"
	"github.com/JetBrains/qodana-cli/v2024/core/corescan"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/JetBrains/qodana-cli/v2024/platform/product"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdenv"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/v2024/platform/utils"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func getPropertiesMap(
	prefix string,
	dotNet qdyaml.DotNet,
	deviceIdSalt []string,
	plugins []string,
	analysisId string,
	coverageDir string,
) map[string]string {
	properties := map[string]string{
		"-Didea.headless.enable.statistics":    strconv.FormatBool(cloud.Token.IsAllowedToSendFUS()),
		"-Didea.headless.statistics.device.id": deviceIdSalt[0],
		"-Didea.headless.statistics.salt":      deviceIdSalt[1],
		"-Dqodana.automation.guid":             utils.QuoteIfSpace(analysisId),
		"-XX:MaxRAMPercentage":                 "70", //only in docker?
	}
	if coverageDir != "" {
		properties["-Dqodana.coverage.input"] = utils.QuoteIfSpace(coverageDir)
	}
	if len(plugins) > 0 {
		properties["-Didea.required.plugins.id"] = strings.Join(plugins, ",")
	}
	if prefix == "Rider" {
		if dotNet.Project != "" {
			properties["-Dqodana.net.project"] = utils.QuoteIfSpace(dotNet.Project)
		} else if dotNet.Solution != "" {
			properties["-Dqodana.net.solution"] = utils.QuoteIfSpace(dotNet.Solution)
		}
		if dotNet.Configuration != "" {
			properties["-Dqodana.net.configuration"] = utils.QuoteIfSpace(dotNet.Configuration)
		}
		if dotNet.Platform != "" {
			properties["-Dqodana.net.platform"] = utils.QuoteIfSpace(dotNet.Platform)
		}
		if dotNet.Frameworks != "" {
			properties["-Dqodana.net.targetFrameworks"] = utils.QuoteIfSpace(dotNet.Frameworks)
		} else if qdenv.IsContainer() {
			// We don't want to scan .NET Framework projects in Linux containers
			properties["-Dqodana.net.targetFrameworks"] = "!net48;!net472;!net471;!net47;!net462;!net461;!net46;!net452;!net451;!net45;!net403;!net40;!net35;!net20;!net11"
		}
	}

	log.Debugf("properties: %v", properties)

	return properties
}

// Common part for installPlugins and qodana executuion
func GetCommonProperties(c corescan.Context) []string {
	systemDir := filepath.Join(c.CacheDir(), "idea", c.Prod().GetVersionBranch())
	pluginsDir := filepath.Join(c.CacheDir(), "plugins", c.Prod().GetVersionBranch())
	lines := []string{
		fmt.Sprintf("-Didea.config.path=%s", utils.QuoteIfSpace(c.ConfigDir())),
		fmt.Sprintf("-Didea.system.path=%s", utils.QuoteIfSpace(systemDir)),
		fmt.Sprintf("-Didea.plugins.path=%s", utils.QuoteIfSpace(pluginsDir)),
		fmt.Sprintf("-Didea.log.path=%s", utils.QuoteIfSpace(c.LogDir())),
	}
	treatAsRelease := os.Getenv(qdenv.QodanaTreatAsRelease)
	if treatAsRelease == "true" {
		lines = append(lines, "-Deap.require.license=release")
	}

	return lines
}

func GetInstallPluginsProperties(c corescan.Context) []string {
	lines := GetCommonProperties(c)

	lines = append(
		lines,
		"-Didea.headless.enable.statistics=false",
		"-Dqodana.application=true",
		"-Dintellij.platform.load.app.info.from.resources=true",
		fmt.Sprintf("-Dqodana.build.number=%s-%s", c.Prod().IdeCode, c.Prod().Build),
	)

	sort.Strings(lines)
	return lines
}

// GetScanProperties writes key=value `props` to file `f` having later key occurrence win
func GetScanProperties(c corescan.Context) []string {
	yaml := c.QodanaYaml()
	yamlProps := yaml.Properties
	dotNetOptions := yaml.DotNet
	plugins := getPluginIds(yaml.Plugins)

	lines := GetCommonProperties(c)

	lines = append(
		lines,
		fmt.Sprintf("-Xlog:gc*:%s", utils.QuoteIfSpace(filepath.Join(c.LogDir(), "gc.log"))),
	)

	if c.JvmDebugPort() > 0 {
		lines = append(
			lines,
			fmt.Sprintf("-agentlib:jdwp=transport=dt_socket,server=y,suspend=y,address=*:%s", containerJvmDebugPort),
		)
	}

	customPluginPathsValue := getCustomPluginPaths(c.Prod())
	if customPluginPathsValue != "" {
		lines = append(lines, fmt.Sprintf("-Dplugin.path=%s", customPluginPathsValue))
	}

	cliProps, flags := c.PropertiesAndFlags()
	for _, f := range flags {
		if f != "" && !utils.Contains(lines, f) {
			lines = append(lines, f)
		}
	}

	props := getPropertiesMap(
		c.Prod().ParentPrefix(),
		dotNetOptions,
		platform.GetDeviceIdSalt(),
		plugins,
		c.AnalysisId(),
		c.CoverageDir(),
	)
	for k, v := range yamlProps { // qodana.yaml – overrides vmoptions
		if !strings.HasPrefix(k, "-") {
			k = fmt.Sprintf("-D%s", k)
		}
		props[k] = v
	}
	for k, v := range cliProps { // CLI – overrides anything
		if !strings.HasPrefix(k, "-") {
			k = fmt.Sprintf("-D%s", k)
		}
		props[k] = v
	}

	for k, v := range props {
		lines = append(lines, fmt.Sprintf("%s=%s", k, v))
	}

	sort.Strings(lines)

	return lines
}

func getCustomPluginPaths(prod product.Product) string {
	path := prod.CustomPluginsPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return ""
	}

	files, err := os.ReadDir(path)
	if err != nil {
		log.Fatal(err)
	}
	var paths []string
	for _, file := range files {
		paths = append(paths, filepath.Join(path, file.Name()))
	}
	return strings.Join(paths, ",")
}

// writeProperties writes the given key=value `props` to file `f` (sets the environment variable)
func writeProperties(c corescan.Context) { // opts.confDirPath(Prod().version)  opts.vmOptionsPath(Prod().version)
	properties := GetScanProperties(c)
	err := os.WriteFile(c.VmOptionsPath(), []byte(strings.Join(properties, "\n")), 0o644)
	if err != nil {
		log.Fatal(err)
	}
	err = os.Setenv(c.Prod().VmOptionsEnv(), c.VmOptionsPath())
	if err != nil {
		log.Fatal(err)
	}
}

func setInstallPluginsVmoptions(c corescan.Context) {
	vmOptions := GetInstallPluginsProperties(c)
	log.Debugf("install plugins options:%s", vmOptions)
	err := os.WriteFile(c.InstallPluginsVmOptionsPath(), []byte(strings.Join(vmOptions, "\n")), 0o644)
	if err != nil {
		log.Fatal(err)
	}
	err = os.Setenv(c.Prod().VmOptionsEnv(), c.InstallPluginsVmOptionsPath())
	if err != nil {
		log.Fatal(err)
	}
}

func getPluginIds(plugins []qdyaml.Plugin) []string {
	ids := make([]string, len(plugins))
	for i, plugin := range plugins {
		ids[i] = plugin.Id
	}
	return ids
}
