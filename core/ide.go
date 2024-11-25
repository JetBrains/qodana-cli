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
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	cp "github.com/otiai10/copy"
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
		if res < platform.QodanaSuccessExitCode || res > platform.QodanaFailThresholdExitCode {
			log.Printf("Wrong exitCode in sarif: %d", res)
			return 1
		}
		log.Printf("IDE exit code: %d", res)
		return res
	}
	log.Printf("IDE process exit code: %d", c)
	return c
}

func runQodanaLocal(opts *QodanaOptions) (int, error) {
	args := getIdeRunCommand(opts)
	ideProcess, err := platform.RunCmdWithTimeout(
		"",
		os.Stdout, os.Stderr,
		opts.GetAnalysisTimeout(),
		platform.QodanaTimeoutExitCodePlaceholder,
		args...,
	)
	res := getIdeExitCode(opts.ResultsDir, ideProcess)
	if res > platform.QodanaSuccessExitCode && res != platform.QodanaFailThresholdExitCode {
		postAnalysis(opts)
		return res, err
	}

	saveReport(opts)
	postAnalysis(opts)
	return res, err
}

func getIdeRunCommand(opts *QodanaOptions) []string {
	args := []string{platform.QuoteIfSpace(Prod.IdeScript)}
	if !Prod.is242orNewer() {
		args = append(args, "inspect")
	}
	args = append(args, "qodana")

	args = append(args, GetIdeArgs(opts)...)
	args = append(args, platform.QuoteIfSpace(opts.ProjectDir), platform.QuoteIfSpace(opts.ResultsDir))
	return args
}

// GetIdeArgs returns qodana command options.
func GetIdeArgs(opts *QodanaOptions) []string {
	arguments := make([]string, 0)
	if opts.ConfigName != "" {
		arguments = append(arguments, "--config", platform.QuoteForWindows(opts.ConfigName))
	}
	if opts.Linter != "" && opts.SaveReport {
		arguments = append(arguments, "--save-report")
	}
	if opts.SourceDirectory != "" {
		arguments = append(arguments, "--source-directory", platform.QuoteForWindows(opts.SourceDirectory))
	}
	if opts.DisableSanity {
		arguments = append(arguments, "--disable-sanity")
	}
	if opts.ProfileName != "" {
		arguments = append(arguments, "--profile-name", platform.QuoteIfSpace(opts.ProfileName))
	}
	if opts.ProfilePath != "" {
		arguments = append(arguments, "--profile-path", platform.QuoteForWindows(opts.ProfilePath))
	}
	if opts.RunPromo != "" {
		arguments = append(arguments, "--run-promo", opts.RunPromo)
	}
	if opts.Script != "" && opts.Script != "default" {
		arguments = append(arguments, "--script", opts.Script)
	}
	if opts.Baseline != "" {
		arguments = append(arguments, "--baseline", platform.QuoteForWindows(opts.Baseline))
	}
	if opts.BaselineIncludeAbsent {
		arguments = append(arguments, "--baseline-include-absent")
	}
	if opts.FailThreshold != "" {
		arguments = append(arguments, "--fail-threshold", opts.FailThreshold)
	}

	if opts.fixesSupported() {
		if opts.FixesStrategy != "" {
			switch strings.ToLower(opts.FixesStrategy) {
			case "apply":
				opts.ApplyFixes = true
			case "cleanup":
				opts.Cleanup = true
			default:
				break
			}
		}
		if opts.Ide != "" && Prod.is233orNewer() {
			if opts.ApplyFixes {
				arguments = append(arguments, "--apply-fixes")
			} else if opts.Cleanup {
				arguments = append(arguments, "--cleanup")
			}
		} else { // remove this block in 2023.3 or later
			if opts.ApplyFixes {
				arguments = append(arguments, "--fixes-strategy", "apply")
			} else if opts.Cleanup {
				arguments = append(arguments, "--fixes-strategy", "cleanup")
			}
		}
	}

	prod := opts.guessProduct() // TODO : think how it could be better handled in presence of random 3rd party linters
	if prod == platform.QDNETC || prod == platform.QDCL {
		// third party common options
		if opts.NoStatistics {
			arguments = append(arguments, "--no-statistics")
		}
		if prod == platform.QDNETC {
			// cdnet options
			if opts.CdnetSolution != "" {
				arguments = append(arguments, "--solution", platform.QuoteForWindows(opts.CdnetSolution))
			}
			if opts.CdnetProject != "" {
				arguments = append(arguments, "--project", platform.QuoteForWindows(opts.CdnetProject))
			}
			if opts.CdnetConfiguration != "" {
				arguments = append(arguments, "--configuration", opts.CdnetConfiguration)
			}
			if opts.CdnetPlatform != "" {
				arguments = append(arguments, "--platform", opts.CdnetPlatform)
			}
			if opts.CdnetNoBuild {
				arguments = append(arguments, "--no-build")
			}
		} else {
			// clang options
			if opts.ClangCompileCommands != "" {
				arguments = append(arguments, "--compile-commands", platform.QuoteForWindows(opts.ClangCompileCommands))
			}
			if opts.ClangArgs != "" {
				arguments = append(arguments, "--clang-args", opts.ClangArgs)
			}
		}
	}

	if opts.Ide == "" {
		if startHash, err := opts.StartHash(); startHash != "" && err == nil && opts.Script == "default" {
			arguments = append(arguments, "--diff-start", startHash)
		}
		if opts.DiffEnd != "" && opts.Script == "default" {
			arguments = append(arguments, "--diff-end", opts.DiffEnd)
		}
		if opts.ForceLocalChangesScript && opts.Script == "default" {
			arguments = append(arguments, "--force-local-changes-script")
		}

		if opts.AnalysisId != "" {
			arguments = append(arguments, "--analysis-id", opts.AnalysisId)
		}

		if opts.CoverageDir != "" {
			arguments = append(arguments, "--coverage-dir", opts.CoverageDir)
		}

		if opts.JvmDebugPort > 0 {
			arguments = append(arguments, "--jvm-debug-port", strconv.Itoa(opts.JvmDebugPort))
		}

		for _, property := range opts.Property {
			arguments = append(arguments, "--property="+property)
		}
	}

	return arguments
}

// postAnalysis post-analysis stage: wait for FUS stats to upload
func postAnalysis(opts *QodanaOptions) {
	err := syncIdeaCache(opts.ProjectDir, opts.CacheDir, true)
	if err != nil {
		log.Warnf("failed to sync .idea directory: %v", err)
	}
	syncConfigCache(opts, false)
	for i := 1; i <= 600; i++ {
		if findProcess("statistics-uploader") {
			time.Sleep(time.Second)
		} else {
			break
		}
	}
}

var ( // base script name
	idea      = "idea"
	phpStorm  = "phpstorm"
	webStorm  = "webstorm"
	rider     = "rider"
	pyCharm   = "pycharm"
	rubyMine  = "rubymine"
	goLand    = "goland"
	rustRover = "rustrover"
	clion     = "clion"
)

var supportedIdes = [...]string{
	idea,
	phpStorm,
	webStorm,
	rider,
	pyCharm,
	rubyMine,
	goLand,
	rustRover,
	clion,
}

func toQodanaCode(baseProduct string) string {
	switch baseProduct {
	case "IC":
		return platform.QDJVMC
	case "PC":
		return platform.QDPYC
	case "IU":
		return platform.QDJVM
	case "PS":
		return platform.QDPHP
	case "WS":
		return platform.QDJS
	case "RD":
		return platform.QDNET
	case "PY":
		return platform.QDPY
	case "GO":
		return platform.QDGO
	case "RM":
		return platform.QDRUBY
	case "RR":
		return platform.QDRST
	case "CL":
		return platform.QDCPP
	default:
		return "QD"
	}
}

func findIde(dir string) string {
	for _, element := range supportedIdes {
		if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("%s%s", element, getScriptSuffix()))); err == nil {
			return element
		}
	}
	return ""
}

func prepareLocalIdeSettings(opts *QodanaOptions) {
	guessProduct(opts)
	if Prod.BaseScriptName == "" {
		log.Fatal("IDE to run is not found")
	}

	platform.ExtractQodanaEnvironment(platform.SetEnv)
	requiresToken := opts.RequiresToken(Prod.EAP || Prod.IsCommunity())
	cloud.SetupLicenseToken(opts.LoadToken(false, requiresToken, true))
	SetupLicenseAndProjectHash(cloud.GetCloudApiEndpoints(), cloud.Token.Token)
	prepareDirectories(
		opts.CacheDir,
		opts.LogDirPath(),
		opts.ConfDirPath(),
	)
	writeProperties(opts)

	if platform.IsContainer() {
		err := syncIdeaCache(opts.CacheDir, opts.ProjectDir, false)
		if err != nil {
			log.Warnf("failed to sync .idea directory: %v", err)
		}
		syncConfigCache(opts, true)
		createUser("/etc/passwd")
	}
}

func prepareDirectories(cacheDir string, logDir string, confDir string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	userPrefsDir := filepath.Join(homeDir, ".java", ".userPrefs")
	directories := []string{
		cacheDir,
		logDir,
		confDir,
		userPrefsDir,
	}
	if platform.IsContainer() {
		if Prod.BaseScriptName == rider {
			nugetDir := filepath.Join(cacheDir, nuget)
			if err := os.Setenv("NUGET_PACKAGES", nugetDir); err != nil {
				log.Fatal(err)
			}
			directories = append(
				directories,
				nugetDir,
			)
		} else if Prod.BaseScriptName == idea {
			directories = append(
				directories,
				filepath.Join(cacheDir, m2),
			)
			if err = os.Setenv("GRADLE_USER_HOME", filepath.Join(cacheDir, "gradle")); err != nil {
				log.Fatal(err)
			}
		}
	}
	for _, dir := range directories {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				log.Fatal(err)
			}
		}
	}

	writeFileIfNew(filepath.Join(userPrefsDir, "prefs.xml"), userPrefsXml)

	ideaOptions := filepath.Join(confDir, "options")
	if _, err := os.Stat(ideaOptions); os.IsNotExist(err) {
		if err := os.MkdirAll(ideaOptions, 0o755); err != nil {
			log.Fatal(err)
		}
	}

	if //goland:noinspection ALL
	runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		writeFileIfNew(filepath.Join(ideaOptions, "security.xml"), securityXml)
	}

	if Prod.BaseScriptName == idea {
		mavenRootDir := filepath.Join(homeDir, ".m2")
		if _, err = os.Stat(mavenRootDir); os.IsNotExist(err) {
			if err = os.MkdirAll(mavenRootDir, 0o755); err != nil {
				log.Fatal(err)
			}
		}
		writeFileIfNew(filepath.Join(mavenRootDir, "settings.xml"), mavenSettingsXml)
		writeFileIfNew(filepath.Join(ideaOptions, "path.macros.xml"), mavenPathMacroxXml)

		androidSdk := os.Getenv(platform.AndroidSdkRoot)
		if androidSdk != "" && platform.IsContainer() {
			writeFileIfNew(filepath.Join(ideaOptions, "project.default.xml"), androidProjectDefaultXml(androidSdk))
			corettoSdk := os.Getenv(platform.QodanaCorettoSdk)
			if corettoSdk != "" {
				writeFileIfNew(filepath.Join(ideaOptions, "jdk.table.xml"), jdkTableXml(corettoSdk))
			}
		}
	}

	disabledPluginsPathSrc := filepath.Join(Prod.Home, "disabled_plugins.txt")
	disabledPluginsPathDst := filepath.Join(confDir, "disabled_plugins.txt")
	if _, err := os.Stat(disabledPluginsPathSrc); err == nil {
		if err := cp.Copy(disabledPluginsPathSrc, disabledPluginsPathDst); err != nil {
			log.Fatal(err)
		}
	}
}

// installPlugins runs plugin installer for every plugin id in qodana.yaml.
func installPlugins(opts *QodanaOptions, plugins []platform.Plugin) {
	if len(plugins) > 0 {
		setInstallPluginsVmoptions(opts)
	}
	for _, plugin := range plugins {
		log.Printf("Installing plugin %s", plugin.Id)
		if res, err := platform.RunCmd("", platform.QuoteIfSpace(Prod.IdeScript), "installPlugins", platform.QuoteIfSpace(plugin.Id)); res > 0 || err != nil {
			os.Exit(res)
		}
	}
}

func syncConfigCache(opts *QodanaOptions, fromCache bool) {
	if Prod.BaseScriptName == idea {
		jdkTableFile := filepath.Join(opts.ConfDirPath(), "options", "jdk.table.xml")
		cacheFile := filepath.Join(opts.CacheDir, "config", Prod.getVersionBranch(), "jdk.table.xml")
		if fromCache {
			if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
				return
			}
			if _, err := os.Stat(jdkTableFile); os.IsNotExist(err) {
				if err := cp.Copy(cacheFile, jdkTableFile); err != nil {
					log.Fatal(err)
				}
				log.Debugf("SDK table is synced from cache")
			}
		} else {
			if _, err := os.Stat(jdkTableFile); os.IsNotExist(err) {
				log.Debugf("SDK table isnt't stored to cache, file doesn't exist")
			} else {
				if err := cp.Copy(jdkTableFile, cacheFile); err != nil {
					log.Fatal(err)
				}
				log.Debugf("SDK table is stored to cache")
			}
		}
	}
}

// syncIdeaCache sync .idea/ content from cache and back.
func syncIdeaCache(from string, to string, overwrite bool) error {
	copyOptions := cp.Options{
		OnDirExists: func(src, dest string) cp.DirExistsAction {
			if overwrite {
				return cp.Merge
			}
			return cp.Untouchable
		},
		OnSymlink: func(src string) cp.SymlinkAction {
			return cp.Skip
		},
	}
	src := filepath.Join(from, ".idea")
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("source .idea directory does not exist: %s", src)
	}
	dst := filepath.Join(to, ".idea")
	log.Printf("Sync IDE cache from: %s to: %s", src, dst)
	if err := cp.Copy(src, dst, copyOptions); err != nil {
		return fmt.Errorf("failed to sync .idea directory: %w", err)
	}

	return nil
}

//goland:noinspection GoBoolExpressions
func getScriptSuffix() string {
	if runtime.GOOS == "windows" {
		return "64.exe"
	}
	return ""
}
