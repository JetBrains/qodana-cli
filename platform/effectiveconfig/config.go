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

package effectiveconfig

import (
	"errors"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2025/platform/msg"
	"github.com/JetBrains/qodana-cli/v2025/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
	"github.com/JetBrains/qodana-cli/v2025/tooling"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
)

// Files – effective configuration files, constructed by calling config-loader-cli.jar,
// all paths are absolute
// + also profile files are stored in config dir
type Files struct {
	ConfigDir               string
	EffectiveQodanaYamlPath string
	LocalQodanaYamlPath     string
	QodanaConfigJsonPath    string
}

func CreateEffectiveConfigFiles(
	localQodanaYamlFullPath string,
	globalConfigurationsFile string,
	globalConfigId string,
	jrePath string,
	systemDir string,
	effectiveConfigDirName string,
	logDir string,
) (Files, error) {
	if globalConfigId != "" && globalConfigurationsFile == "" {
		return Files{}, fmt.Errorf(
			"global configuration id %s is defined without global cofigurations file",
			globalConfigId,
		)
	}
	if globalConfigurationsFile != "" && globalConfigId == "" {
		return Files{}, fmt.Errorf(
			"global configurations file %s is defined without global configuration id",
			globalConfigurationsFile,
		)
	}

	configLoaderCliJar, err := createConfigLoaderCliJar(systemDir)
	if err != nil {
		return Files{}, err
	}
	defer func(name string) {
		err := os.Remove(name)
		if err != nil {
			log.Warnf("Failed to delete config-loader-cli.jar: %v", err)
		}
	}(configLoaderCliJar)

	effectiveConfigDir := filepath.Join(systemDir, effectiveConfigDirName)
	args, err := configurationLoaderCliArgs(
		jrePath,
		configLoaderCliJar,
		localQodanaYamlFullPath,
		globalConfigurationsFile,
		globalConfigId,
		effectiveConfigDir,
	)
	if err != nil {
		return Files{}, err
	}

	log.Debugf("Creating effective configuration in '%s' directory, args: %v", effectiveConfigDir, args)
	if _, _, res, err := utils.LaunchAndLog(logDir, "config-loader-cli", args...); res > 0 || err != nil {
		if err == nil {
			err = errors.New("failed to create effective configuration")
		} else {
			//goland:noinspection GoErrorStringFormat
			err = fmt.Errorf("Failed to create effective configuration: %v", err)
		}
		msg.ErrorMessage("Failed to create effective configuration. See log above for details")
		return Files{}, err
	}

	effectiveQodanaYamlData, err := getEffectiveQodanaYamlData(effectiveConfigDir)
	if err != nil {
		return Files{}, err
	}

	err = verifyEffectiveQodanaYamlIdeAndLinterMatchLocal(effectiveQodanaYamlData, localQodanaYamlFullPath)
	if err != nil {
		return Files{}, err
	}
	msg.SuccessMessage("Loaded Qodana Configuration")
	return effectiveQodanaYamlData, nil
}

func createConfigLoaderCliJar(systemDir string) (string, error) {
	configLoaderCliJarPath := filepath.Join(systemDir, "tools", "config-loader-cli.jar")
	if isFileExists(configLoaderCliJarPath) {
		err := os.Remove(configLoaderCliJarPath)
		if err != nil {
			return "", fmt.Errorf("failed to delete existing config-loader-cli.jar: %v", err)
		}
	}
	err := os.MkdirAll(filepath.Dir(configLoaderCliJarPath), 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create directory for config-loader-cli.jar: %v", err)
	}
	log.Debugf("creating config-loader-cli.jar at '%s'", configLoaderCliJarPath)
	err = os.WriteFile(configLoaderCliJarPath, tooling.ConfigLoaderCli, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write config-loader-cli.jar content to %s: %v", configLoaderCliJarPath, err)
	}
	return configLoaderCliJarPath, nil
}

func configurationLoaderCliArgs(
	jrePath string,
	configLoaderCliJarPath string,
	localQodanaYamlPath string,
	globalConfigurationsFile string,
	globalConfigId string,
	effectiveConfigDir string,
) ([]string, error) {
	if jrePath == "" {
		return nil, fmt.Errorf("JRE not found. Required for effective configuration creation")
	}
	if configLoaderCliJarPath == "" {
		return nil, fmt.Errorf("config-loader-cli.jar not found. Required for effective configuration creation")
	}

	var err error
	args := []string{
		utils.QuoteIfSpace(utils.QuoteForWindows(jrePath)),
		"-jar",
		utils.QuoteForWindows(configLoaderCliJarPath),
	}

	effectiveConfigDirAbs, err := filepath.Abs(effectiveConfigDir)
	if err != nil {
		err := fmt.Errorf(
			"failed to compute absolute path of effective configuration directory %s: %v",
			effectiveConfigDir,
			err,
		)
		return nil, err
	}
	args = append(args, "--effective-config-out-dir", utils.QuoteForWindows(effectiveConfigDirAbs))

	if localQodanaYamlPath != "" {
		localQodanaYamlPathAbs, err := filepath.Abs(localQodanaYamlPath)
		if err != nil {
			err := fmt.Errorf(
				"failed to compute absolute path of local qodana.yaml file %s: %v",
				localQodanaYamlPath,
				err,
			)
			return nil, err
		}
		args = append(args, "--local-qodana-yaml", utils.QuoteIfSpace(utils.QuoteForWindows(localQodanaYamlPathAbs)))
	}

	if globalConfigurationsFile != "" {
		globalConfigurationsFileAbs, err := filepath.Abs(globalConfigurationsFile)
		if err != nil {
			err := fmt.Errorf(
				"failed to compute absolute path of global configurations file %s: %v",
				globalConfigurationsFile,
				err,
			)
			return nil, err
		}
		args = append(
			args,
			"--global-configs-file",
			utils.QuoteIfSpace(utils.QuoteForWindows(globalConfigurationsFileAbs)),
		)
	}
	if globalConfigId != "" {
		args = append(args, "--global-config-id", utils.QuoteIfSpace(utils.QuoteForWindows(globalConfigId)))
	}
	return args, nil
}

func getEffectiveQodanaYamlData(effectiveConfigDir string) (Files, error) {
	effectiveQodanaYamlPath := filepath.Join(effectiveConfigDir, "effective.qodana.yaml")
	if !isFileExists(effectiveQodanaYamlPath) {
		effectiveQodanaYamlPath = ""
	}
	localQodanaYamlPath := filepath.Join(effectiveConfigDir, "qodana.yaml")
	if !isFileExists(localQodanaYamlPath) {
		localQodanaYamlPath = ""
	}
	qodanaConfigJsonPath := filepath.Join(effectiveConfigDir, "qodana-config.json")
	if !isFileExists(qodanaConfigJsonPath) {
		qodanaConfigJsonPath = ""
	}

	if effectiveQodanaYamlPath != "" && qodanaConfigJsonPath == "" {
		return Files{}, errors.New("effective.qodana.yaml file doesn't have a qodana-config.json file")
	}
	if localQodanaYamlPath != "" && effectiveQodanaYamlPath == "" {
		return Files{}, errors.New("local qodana.yaml file doesn't have an effective.qodana.yaml file")
	}
	return Files{
		ConfigDir:               effectiveConfigDir,
		EffectiveQodanaYamlPath: effectiveQodanaYamlPath,
		LocalQodanaYamlPath:     localQodanaYamlPath,
		QodanaConfigJsonPath:    qodanaConfigJsonPath,
	}, nil
}

func isFileExists(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	} else if os.IsNotExist(err) {
		return false
	} else {
		log.Fatalf("Failed to verify existence of file %s: %s", path, err)
	}
	return false
}

func verifyEffectiveQodanaYamlIdeAndLinterMatchLocal(
	effectiveQodanaYamlData Files,
	localQodanaYamlPathFromRoot string,
) error {
	effectiveYaml := qdyaml.LoadQodanaYamlByFullPath(effectiveQodanaYamlData.EffectiveQodanaYamlPath)
	effectiveLinter := effectiveYaml.Linter
	effectiveIde := effectiveYaml.Ide
	if effectiveLinter == "" && effectiveIde == "" {
		return nil
	}

	isLocalQodanaYamlPresent := effectiveQodanaYamlData.LocalQodanaYamlPath != ""
	if isLocalQodanaYamlPresent {
		localQodanaYaml := qdyaml.LoadQodanaYamlByFullPath(effectiveQodanaYamlData.LocalQodanaYamlPath)

		failedToCreateEffectiveConfigurationMessage := "Failed to create effective configuration"

		topMessageTemplate := "'%s: %s' is specified in one of files provided by 'imports' from " + localQodanaYamlPathFromRoot + " '%s' is required in root qodana.yaml"
		bottomMessageTemplate := "Add `ide: %s` to " + localQodanaYamlPathFromRoot
		if effectiveIde != localQodanaYaml.Ide {
			msg.ErrorMessage(failedToCreateEffectiveConfigurationMessage)
			msg.ErrorMessage(topMessageTemplate, "ide", effectiveIde, "ide")
			msg.ErrorMessage(bottomMessageTemplate, effectiveIde)
			return errors.New("effective.qodana.yaml `ide` doesn't match root qodana.yaml `ide`")
		}
		//goland:noinspection GoDfaConstantCondition
		if effectiveLinter != localQodanaYaml.Linter {
			msg.ErrorMessage(failedToCreateEffectiveConfigurationMessage)
			msg.ErrorMessage(topMessageTemplate, "linter", effectiveLinter, "linter")
			msg.ErrorMessage(bottomMessageTemplate, effectiveLinter)
			return errors.New("effective.qodana.yaml `linter` doesn't match root qodana.yaml `linter`")
		}
	}
	return nil
}
