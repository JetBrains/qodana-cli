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
	"github.com/JetBrains/qodana-cli/v2024/platform"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func getPropertiesMap(
	prefix string,
	dotNet platform.DotNet,
	deviceIdSalt []string,
	plugins []string,
	analysisId string,
	coverageDir string,
) map[string]string {
	properties := map[string]string{
		"-Didea.headless.enable.statistics":    strconv.FormatBool(cloud.Token.IsAllowedToSendFUS()),
		"-Didea.headless.statistics.device.id": deviceIdSalt[0],
		"-Didea.headless.statistics.salt":      deviceIdSalt[1],
		"-Dqodana.automation.guid":             platform.QuoteIfSpace(analysisId),
		"-XX:MaxRAMPercentage":                 "70", //only in docker?
	}
	if coverageDir != "" {
		properties["-Dqodana.coverage.input"] = platform.QuoteIfSpace(coverageDir)
	}
	if len(plugins) > 0 {
		properties["-Didea.required.plugins.id"] = strings.Join(plugins, ",")
	}
	if prefix == "Rider" {
		if dotNet.Project != "" {
			properties["-Dqodana.net.project"] = platform.QuoteIfSpace(dotNet.Project)
		} else if dotNet.Solution != "" {
			properties["-Dqodana.net.solution"] = platform.QuoteIfSpace(dotNet.Solution)
		}
		if dotNet.Configuration != "" {
			properties["-Dqodana.net.configuration"] = platform.QuoteIfSpace(dotNet.Configuration)
		}
		if dotNet.Platform != "" {
			properties["-Dqodana.net.platform"] = platform.QuoteIfSpace(dotNet.Platform)
		}
		if dotNet.Frameworks != "" {
			properties["-Dqodana.net.targetFrameworks"] = platform.QuoteIfSpace(dotNet.Frameworks)
		} else if platform.IsContainer() {
			// We don't want to scan .NET Framework projects in Linux containers
			properties["-Dqodana.net.targetFrameworks"] = "!net48;!net472;!net471;!net47;!net462;!net461;!net46;!net452;!net451;!net45;!net403;!net40;!net35;!net20;!net11"
		}
	}

	log.Debugf("properties: %v", properties)

	return properties
}

// GetCommonProperties computes common properties for installPlugins and qodana executuion
func GetCommonProperties(opts *QodanaOptions) []string {
	systemDir := filepath.Join(opts.CacheDir, "idea", Prod.getVersionBranch())
	pluginsDir := filepath.Join(opts.CacheDir, "plugins", Prod.getVersionBranch())
	lines := []string{
		fmt.Sprintf("-Didea.config.path=%s", platform.QuoteIfSpace(opts.ConfDirPath())),
		fmt.Sprintf("-Didea.system.path=%s", platform.QuoteIfSpace(systemDir)),
		fmt.Sprintf("-Didea.plugins.path=%s", platform.QuoteIfSpace(pluginsDir)),
		fmt.Sprintf("-Didea.log.path=%s", platform.QuoteIfSpace(opts.LogDirPath())),
	}
	treatAsRelease := os.Getenv(platform.QodanaTreatAsRelease)
	if treatAsRelease == "true" {
		lines = append(lines, "-Deap.require.license=release")
	}

	return lines
}

func GetInstallPluginsProperties(opts *QodanaOptions) []string {
	lines := GetCommonProperties(opts)

	lines = append(lines,
		"-Didea.headless.enable.statistics=false",
		"-Dqodana.application=true",
		"-Dintellij.platform.load.app.info.from.resources=true",
		fmt.Sprintf("-Dqodana.build.number=%s-%s", Prod.IDECode, Prod.Build),
	)

	sort.Strings(lines)
	return lines
}

// GetScanProperties writes key=value `props` to file `f` having later key occurrence win
func GetScanProperties(opts *QodanaOptions, yamlProps map[string]string, dotNetOptions platform.DotNet, plugins []string) []string {
	lines := GetCommonProperties(opts)

	lines = append(
		lines,
		fmt.Sprintf("-Xlog:gc*:%s", platform.QuoteIfSpace(filepath.Join(opts.LogDirPath(), "gc.log"))),
	)

	if opts.JvmDebugPort > 0 {
		lines = append(lines, fmt.Sprintf("-agentlib:jdwp=transport=dt_socket,server=y,suspend=y,address=*:%s", containerJvmDebugPort))
	}

	customPluginPathsValue := getCustomPluginPaths()
	if customPluginPathsValue != "" {
		lines = append(lines, fmt.Sprintf("-Dplugin.path=%s", customPluginPathsValue))
	}

	cliProps, flags := opts.Properties()
	for _, f := range flags {
		if f != "" && !platform.Contains(lines, f) {
			lines = append(lines, f)
		}
	}

	props := getPropertiesMap(
		Prod.parentPrefix(),
		dotNetOptions,
		platform.GetDeviceIdSalt(),
		plugins,
		opts.AnalysisId,
		opts.CoverageDirPath(),
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

func getCustomPluginPaths() string {
	path := Prod.CustomPluginsPath()
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
func writeProperties(opts *QodanaOptions) { // opts.confDirPath(Prod.Version)  opts.vmOptionsPath(Prod.Version)
	properties := GetScanProperties(opts, opts.QdConfig.Properties, opts.QdConfig.DotNet, getPluginIds(opts.QdConfig.Plugins))
	err := os.WriteFile(opts.vmOptionsPath(), []byte(strings.Join(properties, "\n")), 0o644)
	if err != nil {
		log.Fatal(err)
	}
	err = os.Setenv(Prod.vmOptionsEnv(), opts.vmOptionsPath())
	if err != nil {
		log.Fatal(err)
	}
}

func setInstallPluginsVmoptions(opts *QodanaOptions) {
	vmOptions := GetInstallPluginsProperties(opts)
	log.Debugf("install plugins options:%s", vmOptions)
	err := os.WriteFile(opts.installPluginsVmOptionsPath(), []byte(strings.Join(vmOptions, "\n")), 0o644)
	if err != nil {
		log.Fatal(err)
	}
	err = os.Setenv(Prod.vmOptionsEnv(), opts.installPluginsVmOptionsPath())
	if err != nil {
		log.Fatal(err)
	}
}
