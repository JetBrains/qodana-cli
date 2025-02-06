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
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/JetBrains/qodana-cli/v2024/platform/scan"
	"github.com/JetBrains/qodana-cli/v2024/platform/scan/startup"
	"github.com/JetBrains/qodana-cli/v2024/platform/scan/startup/product"
	"github.com/JetBrains/qodana-cli/v2024/platform/utils"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// getIdeExitCode gets IDEA "exitCode" from SARIF.
func getIdeExitCode(resultsDir string, c int) (res int) {
	if c != 0 {
		return c
	}
	s, err := platform.ReadReport(filepath.Join(resultsDir, "qodana-short.sarif.json"))
	if err != nil {
		log.Fatal(err)
	}
	if len(s.Runs) > 0 && len(s.Runs[0].Invocations) > 0 {
		res := int(s.Runs[0].Invocations[0].ExitCode)
		if res < utils.QodanaSuccessExitCode || res > utils.QodanaFailThresholdExitCode {
			log.Printf("Wrong exitCode in sarif: %d", res)
			return 1
		}
		log.Printf("IDE exit code: %d", res)
		return res
	}
	log.Printf("IDE process exit code: %d", c)
	return c
}

func runQodanaLocal(c scan.Context) (int, error) {
	writeProperties(c)
	args := getIdeRunCommand(c)
	ideProcess, err := utils.RunCmdWithTimeout(
		"",
		os.Stdout, os.Stderr,
		c.GetAnalysisTimeout(),
		utils.QodanaTimeoutExitCodePlaceholder,
		args...,
	)
	res := getIdeExitCode(c.ResultsDir, ideProcess)
	if res > utils.QodanaSuccessExitCode && res != utils.QodanaFailThresholdExitCode {
		postAnalysis(c)
		return res, err
	}

	saveReport(c)
	postAnalysis(c)
	return res, err
}

func getIdeRunCommand(c scan.Context) []string {
	args := []string{utils.QuoteIfSpace(c.Prod.IdeScript)}
	if !c.Prod.Is242orNewer() {
		args = append(args, "inspect")
	}
	args = append(args, "qodana")

	args = append(args, GetIdeArgs(c)...)
	args = append(args, utils.QuoteIfSpace(c.ProjectDir), utils.QuoteIfSpace(c.ResultsDir))
	return args
}

// GetIdeArgs returns qodana command options.
func GetIdeArgs(c scan.Context) []string {
	arguments := make([]string, 0)
	if c.ConfigName != "" {
		arguments = append(arguments, "--config", utils.QuoteForWindows(c.ConfigName))
	}
	if c.Linter != "" && c.SaveReport {
		arguments = append(arguments, "--save-report")
	}
	if c.SourceDirectory != "" {
		arguments = append(arguments, "--source-directory", utils.QuoteForWindows(c.SourceDirectory))
	}
	if c.DisableSanity {
		arguments = append(arguments, "--disable-sanity")
	}
	if c.ProfileName != "" {
		arguments = append(arguments, "--profile-name", utils.QuoteIfSpace(c.ProfileName))
	}
	if c.ProfilePath != "" {
		arguments = append(arguments, "--profile-path", utils.QuoteForWindows(c.ProfilePath))
	}
	if c.RunPromo != "" {
		arguments = append(arguments, "--run-promo", c.RunPromo)
	}
	if c.Script != "" && c.Script != "default" {
		arguments = append(arguments, "--script", c.Script)
	}
	if c.Baseline != "" {
		arguments = append(arguments, "--baseline", utils.QuoteForWindows(c.Baseline))
	}
	if c.BaselineIncludeAbsent {
		arguments = append(arguments, "--baseline-include-absent")
	}
	if c.FailThreshold != "" {
		arguments = append(arguments, "--fail-threshold", c.FailThreshold)
	}

	if c.FixesSupported() {
		applyFixes := c.ApplyFixes
		if c.FixesStrategy != "" {
			switch strings.ToLower(c.FixesStrategy) {
			case "apply":
				applyFixes = true
			case "cleanup":
				c.Cleanup = true
			default:
				break
			}
		}
		if c.Ide != "" && c.Prod.Is233orNewer() {
			if applyFixes {
				arguments = append(arguments, "--apply-fixes")
			} else if c.Cleanup {
				arguments = append(arguments, "--cleanup")
			}
		} else { // remove this block in 2023.3 or later
			if applyFixes {
				arguments = append(arguments, "--fixes-strategy", "apply")
			} else if c.Cleanup {
				arguments = append(arguments, "--fixes-strategy", "cleanup")
			}
		}
	}

	prod := product.GuessProductCode(
		c.Ide,
		c.Linter,
	) // TODO : think how it could be better handled in presence of random 3rd party linters
	if prod == product.QDNETC || prod == product.QDCL {
		// third party common options
		if c.NoStatistics {
			arguments = append(arguments, "--no-statistics")
		}
		if prod == product.QDNETC {
			// cdnet options
			if c.CdnetSolution != "" {
				arguments = append(arguments, "--solution", utils.QuoteForWindows(c.CdnetSolution))
			}
			if c.CdnetProject != "" {
				arguments = append(arguments, "--project", utils.QuoteForWindows(c.CdnetProject))
			}
			if c.CdnetConfiguration != "" {
				arguments = append(arguments, "--configuration", c.CdnetConfiguration)
			}
			if c.CdnetPlatform != "" {
				arguments = append(arguments, "--platform", c.CdnetPlatform)
			}
			if c.CdnetNoBuild {
				arguments = append(arguments, "--no-build")
			}
		} else {
			// clang options
			if c.ClangCompileCommands != "" {
				arguments = append(arguments, "--compile-commands", utils.QuoteForWindows(c.ClangCompileCommands))
			}
			if c.ClangArgs != "" {
				arguments = append(arguments, "--clang-args", c.ClangArgs)
			}
		}
	}

	if c.Ide == "" {
		if startHash, err := c.StartHash(); startHash != "" && err == nil && c.Script == "default" {
			arguments = append(arguments, "--diff-start", startHash)
		}
		if c.DiffEnd != "" && c.Script == "default" {
			arguments = append(arguments, "--diff-end", c.DiffEnd)
		}
		if c.ForceLocalChangesScript && c.Script == "default" {
			arguments = append(arguments, "--force-local-changes-script")
		}

		if c.AnalysisId != "" {
			arguments = append(arguments, "--analysis-id", c.AnalysisId)
		}

		if c.CoverageDir != "" {
			arguments = append(arguments, "--coverage-dir", c.CoverageDir)
		}

		if c.JvmDebugPort > 0 {
			arguments = append(arguments, "--jvm-debug-port", strconv.Itoa(c.JvmDebugPort))
		}

		for _, property := range c.Property() {
			arguments = append(arguments, "--property="+property)
		}
	}
	return arguments
}

// postAnalysis post-analysis stage: wait for FUS stats to upload
func postAnalysis(c scan.Context) {
	err := startup.SyncIdeaCache(c.ProjectDir, c.CacheDir, true)
	if err != nil {
		log.Warnf("failed to sync .idea directory: %v", err)
	}
	startup.SyncConfigCache(c.Prod, c.ConfigDir, c.CacheDir, false)
	for i := 1; i <= 600; i++ {
		if utils.FindProcess("statistics-uploader") {
			time.Sleep(time.Second)
		} else {
			break
		}
	}
}

// installPlugins runs plugin installer for every plugin id in qodana.yaml.
func installPlugins(c scan.Context) {
	if !c.IsNative() {
		return
	}

	plugins := c.QodanaYaml.Plugins
	if len(plugins) > 0 {
		setInstallPluginsVmoptions(c)
	}
	for _, plugin := range plugins {
		log.Printf("Installing plugin %s", plugin.Id)
		if res, err := utils.RunCmd(
			"",
			utils.QuoteIfSpace(c.Prod.IdeScript),
			"installPlugins",
			utils.QuoteIfSpace(plugin.Id),
		); res > 0 || err != nil {
			os.Exit(res)
		}
	}
}
