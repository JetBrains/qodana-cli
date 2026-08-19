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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/JetBrains/qodana-cli/internal/cloud"
	"github.com/JetBrains/qodana-cli/internal/core/corescan"
	"github.com/JetBrains/qodana-cli/internal/foundation/str"
	"github.com/JetBrains/qodana-cli/internal/platform"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/JetBrains/qodana-cli/internal/platform/qdyaml"
	log "github.com/sirupsen/logrus"
)

func getScanPropertiesMap(
	prefix string,
	dotNet qdyaml.DotNet,
	deviceIdSalt []string,
	plugins []string,
	analysisId string,
	coverageDir string,
	repositoryRoot string,
) map[string]string {
	properties := map[string]string{
		"-Didea.headless.enable.statistics":    strconv.FormatBool(cloud.Token.IsAllowedToSendFUS()),
		"-Didea.headless.statistics.device.id": deviceIdSalt[0],
		"-Didea.headless.statistics.salt":      deviceIdSalt[1],
		"-Dqodana.automation.guid":             str.QuoteIfSpace(analysisId),
		"-XX:MaxRAMPercentage":                 "70", //only in docker?
	}
	if coverageDir != "" {
		properties["-Dqodana.coverage.input"] = str.QuoteIfSpace(coverageDir)
	}
	if repositoryRoot != "" && repositoryRoot != "." {
		properties["-Dqodana.path.to.project.dir.from.project.root"] = str.QuoteIfSpace(repositoryRoot)
	}
	if len(plugins) > 0 {
		properties["-Didea.required.plugins.id"] = strings.Join(plugins, ",")
	}
	if prefix == "Rider" {
		if dotNet.Project != "" {
			properties["-Dqodana.net.project"] = str.QuoteIfSpace(dotNet.Project)
		} else if dotNet.Solution != "" {
			properties["-Dqodana.net.solution"] = str.QuoteIfSpace(dotNet.Solution)
		}
		if dotNet.Configuration != "" {
			properties["-Dqodana.net.configuration"] = str.QuoteIfSpace(dotNet.Configuration)
		}
		if dotNet.Platform != "" {
			properties["-Dqodana.net.platform"] = str.QuoteIfSpace(dotNet.Platform)
		}
		if dotNet.Frameworks != "" {
			properties["-Dqodana.net.targetFrameworks"] = str.QuoteIfSpace(dotNet.Frameworks)
		} else if qdenv.IsContainer() {
			// We don't want to scan .NET Framework projects in Linux containers
			properties["-Dqodana.net.targetFrameworks"] = "!net48;!net472;!net471;!net47;!net462;!net461;!net46;!net452;!net451;!net45;!net403;!net40;!net35;!net20;!net11"
		}
	}

	log.Debugf("properties: %v", properties)

	return properties
}

type propertyMaps struct {
	common        map[string]string
	scan          map[string]string
	yamlOverrides map[string]string
	cliOverrides  map[string]string
	flags         []string
}

func createPropertyMaps(c corescan.Context) propertyMaps {
	yaml := c.QodanaYamlConfig()
	cliOverrides, flags := c.PropertiesAndFlags()

	return propertyMaps{
		common: getCommonProperties(c),
		scan: getScanPropertiesMap(
			c.Prod().ParentPrefix(),
			yaml.DotNet,
			platform.GetDeviceIdSalt(),
			getPluginIds(yaml.Plugins),
			c.AnalysisId(),
			c.CoverageDir(),
			c.ProjectDirPathRelativeToRepositoryRoot(),
		),
		yamlOverrides: normalizeProperties(yaml.Properties),
		cliOverrides:  normalizeProperties(cliOverrides),
		flags:         flags,
	}
}

func normalizeProperties(properties map[string]string) map[string]string {
	normalized := make(map[string]string, len(properties))
	for key, value := range properties {
		if !strings.HasPrefix(key, "-") {
			key = fmt.Sprintf("-D%s", key)
		}
		normalized[key] = value
	}
	return normalized
}

func mergePropertyMaps(maps ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, properties := range maps {
		for key, value := range properties {
			merged[key] = value
		}
	}
	return merged
}

// getCommonProperties computes common properties for installPlugins and qodana executuion
func getCommonProperties(c corescan.Context) map[string]string {
	systemDir := filepath.Join(c.CacheDir(), "idea", c.Prod().GetVersionBranch())
	pluginsDir := filepath.Join(c.CacheDir(), "plugins", c.Prod().GetVersionBranch())
	properties := map[string]string{
		"-Didea.config.path":  str.QuoteIfSpace(c.ConfigDir()),
		"-Didea.system.path":  str.QuoteIfSpace(systemDir),
		"-Didea.plugins.path": str.QuoteIfSpace(pluginsDir),
		"-Didea.log.path":     str.QuoteIfSpace(c.LogDir()),
	}
	if os.Getenv(qdenv.QodanaTreatAsRelease) == "true" {
		properties["-Deap.require.license"] = "release"
	}

	return properties
}

func GetInstallPluginsProperties(c corescan.Context) []string {
	propertyMaps := createPropertyMaps(c)
	properties := propertyMaps.common
	// if yaml/cli overrides some param, take it into account
	for key, value := range mergePropertyMaps(
		propertyMaps.yamlOverrides,
		propertyMaps.cliOverrides,
	) {
		if _, isCommonProperty := propertyMaps.common[key]; isCommonProperty {
			properties[key] = value
		}
	}
	properties["-Didea.headless.enable.statistics"] = "false"
	properties["-Dqodana.application"] = "true"
	properties["-Dintellij.platform.load.app.info.from.resources"] = "true"
	properties["-Dqodana.build.number"] = fmt.Sprintf("%s-%s", c.Prod().IdeCode, c.Prod().Build)

	lines := make([]string, 0, len(properties))
	for key, value := range properties {
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}

	sort.Strings(lines)
	return lines
}

// GetScanProperties writes key=value `props` to file `f` having later key occurrence win
func GetScanProperties(c corescan.Context) []string {
	propertyMaps := createPropertyMaps(c)

	lines := make([]string, 0)

	lines = append(
		lines,
		fmt.Sprintf("-Xlog:gc*:%s", str.QuoteIfSpace(filepath.Join(c.LogDir(), "gc.log"))),
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
	disabledPluginsFile := c.Prod().DisabledPluginsFilePath()
	if _, err := os.Stat(disabledPluginsFile); err == nil {
		lines = append(lines, fmt.Sprintf("-Ddisabled.plugins.file.path=%s", disabledPluginsFile))
	}

	for _, f := range propertyMaps.flags {
		if f != "" && !str.Contains(lines, f) {
			lines = append(lines, f)
		}
	}

	props := mergePropertyMaps(
		propertyMaps.common,
		propertyMaps.scan,
		propertyMaps.yamlOverrides,
		propertyMaps.cliOverrides,
	)

	for k, v := range props {
		lines = append(lines, fmt.Sprintf("%s=%s", k, v))
	}

	sort.Strings(lines)

	return lines
}

func getCustomPluginPaths(prod product.Product) string {
	path := prod.CustomPluginsPath()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
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
