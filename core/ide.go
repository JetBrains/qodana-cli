/*
 * Copyright 2021-2023 JetBrains s.r.o.
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
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2023/cloud"
	"github.com/owenrumney/go-sarif/v2/sarif"
	"os"
	"path/filepath"
	"runtime"
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
	s, err := sarif.Open(filepath.Join(resultsDir, "qodana-short.sarif.json"))
	if err != nil {
		log.Fatal(err)
	}
	if len(s.Runs) > 0 && len(s.Runs[0].Invocations) > 0 {
		if tmp := s.Runs[0].Invocations[0].ExitCode; tmp != nil {
			res = *tmp
			if res < QodanaSuccessExitCode || res > QodanaFailThresholdExitCode {
				log.Printf("Wrong exitCode in sarif: %d", res)
				return 1
			}
			log.Printf("IDE exit code: %d", res)
			return res
		}
	}
	log.Printf("IDE process exit code: %d", c)
	return c
}

func runQodanaLocal(opts *QodanaOptions) int {
	args := getIdeRunCommand(opts)
	res := getIdeExitCode(opts.ResultsDir, RunCmd("", args...))
	if res > QodanaSuccessExitCode && res != QodanaFailThresholdExitCode {
		postAnalysis(opts)
		return res
	}
	if opts.SaveReport || opts.ShowReport {
		saveReport(opts)
	}
	postAnalysis(opts)
	return res
}

func getIdeRunCommand(opts *QodanaOptions) []string {
	args := []string{QuoteForWindows(Prod.IdeScript), "inspect", "qodana"}
	args = append(args, getIdeArgs(opts)...)
	args = append(args, QuoteForWindows(opts.ProjectDir), QuoteForWindows(opts.ResultsDir))
	return args
}

// getIdeArgs returns qodana command options.
func getIdeArgs(opts *QodanaOptions) []string {
	arguments := make([]string, 0)
	if opts.Linter != "" && opts.SaveReport {
		arguments = append(arguments, "--save-report")
	}
	if opts.SourceDirectory != "" {
		arguments = append(arguments, "--source-directory", QuoteForWindows(opts.SourceDirectory))
	}
	if opts.DisableSanity {
		arguments = append(arguments, "--disable-sanity")
	}
	if opts.ProfileName != "" {
		arguments = append(arguments, "--profile-name", quoteIfSpace(opts.ProfileName))
	}
	if opts.ProfilePath != "" {
		arguments = append(arguments, "--profile-path", QuoteForWindows(opts.ProfilePath))
	}
	if opts.RunPromo != "" {
		arguments = append(arguments, "--run-promo", opts.RunPromo)
	}
	if opts.Script != "" && opts.Script != "default" {
		arguments = append(arguments, "--script", opts.Script)
	}
	if opts.Baseline != "" {
		arguments = append(arguments, "--baseline", QuoteForWindows(opts.Baseline))
	}
	if opts.BaselineIncludeAbsent {
		arguments = append(arguments, "--baseline-include-absent")
	}
	if opts.FailThreshold != "" {
		arguments = append(arguments, "--fail-threshold", opts.FailThreshold)
	}
	if opts.GitReset && opts.Commit != "" && opts.Script == "default" {
		arguments = append(arguments, "--script", "local-changes")
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

	prod := opts.guessProduct()
	if prod == QDNETC || prod == QDCL {
		// third party common options
		if opts.NoStatistics {
			arguments = append(arguments, "--no-statistics")
		}
		if prod == QDNETC {
			// cdnet options
			if opts.Solution != "" {
				arguments = append(arguments, "--solution", QuoteForWindows(opts.Solution))
			}
			if opts.Project != "" {
				arguments = append(arguments, "--project", QuoteForWindows(opts.Project))
			}
			if opts.Configuration != "" {
				arguments = append(arguments, "--configuration", opts.Configuration)
			}
			if opts.Platform != "" {
				arguments = append(arguments, "--platform", opts.Platform)
			}
			if opts.NoBuild {
				arguments = append(arguments, "--no-build")
			}
		} else {
			// clang options
			if opts.CompileCommands != "" {
				arguments = append(arguments, "--compile-commands", QuoteForWindows(opts.CompileCommands))
			}
			if opts.ClangArgs != "" {
				arguments = append(arguments, "--clang-args", opts.ClangArgs)
			}
		}
	}

	if opts.Ide == "" {
		if opts.AnalysisId != "" {
			arguments = append(arguments, "--analysis-id", opts.AnalysisId)
		}

		if opts.CoverageDir != "" {
			arguments = append(arguments, "--coverage-dir", opts.CoverageDir)
		}

		for _, property := range opts.Property {
			arguments = append(arguments, "--property="+property)
		}
	}

	return arguments
}

// postAnalysis post-analysis stage: wait for FUS stats to upload
func postAnalysis(opts *QodanaOptions) {
	syncIdeaCache(opts.ProjectDir, opts.CacheDir, true)
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
}

func toQodanaCode(baseProduct string) string {
	switch baseProduct {
	case "IC":
		return QDJVMC
	case "PC":
		return QDPYC
	case "IU":
		return QDJVM
	case "PS":
		return QDPHP
	case "WS":
		return QDJS
	case "RD":
		return QDNET
	case "PY":
		return QDPY
	case "GO":
		return QDGO
	case "RM":
		return QDRUBY
	case "RR":
		return QDRST
	default:
		return "QD"
	}
}

func scriptToProductCode(scriptName string) string {
	switch scriptName {
	case idea:
		return QDJVM
	case phpStorm:
		return QDPHP
	case webStorm:
		return QDJS
	case rider:
		return QDNET
	case pyCharm:
		return QDPY
	case rubyMine:
		return QDRUBY
	case goLand:
		return QDGO
	case rustRover:
		return QDRST
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

// readIdeProductInfo returns IDE info from the given path.
func readIdeProductInfo(ideDir string) map[string]interface{} {
	if //goland:noinspection ALL
	runtime.GOOS == "darwin" {
		ideDir = filepath.Join(ideDir, "Resources")
	}
	productInfo := filepath.Join(ideDir, "product-info.json")
	if _, err := os.Stat(productInfo); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	productInfoFile, err := os.ReadFile(productInfo)
	if err != nil {
		log.Printf("Problem loading product-info.json: %v ", err)
		return nil
	}
	var productInfoMap map[string]interface{}
	err = json.Unmarshal(productInfoFile, &productInfoMap)
	if err != nil {
		log.Printf("Not a valid product-info.json: %v ", err)
		return nil
	}
	return productInfoMap
}

func readAppInfoXml(ideDir string) appInfo {
	bytes, _ := os.ReadFile(filepath.Join(ideDir, "bin", qodanaAppInfoFilename))
	var appInfo appInfo
	err := xml.Unmarshal(bytes, &appInfo)
	if err != nil {
		log.Fatal(err)
	}
	return appInfo
}

func prepareLocalIdeSettings(opts *QodanaOptions) {
	guessProduct(opts)
	if Prod.BaseScriptName == "" {
		log.Fatal("IDE to run is not found")
	}

	ExtractQodanaEnvironment(setEnv)
	SetupLicenseToken(opts)
	SetupLicenseAndProjectHash(cloud.Token.Token)
	prepareDirectories(
		opts.CacheDir,
		opts.logDirPath(),
		opts.ConfDirPath(),
	)
	Config = GetQodanaYaml(opts.ProjectDir)
	writeProperties(opts)

	if IsContainer() {
		syncIdeaCache(opts.CacheDir, opts.ProjectDir, false)
		createUser("/etc/passwd")
	}

	bootstrap(Config.Bootstrap, opts.ProjectDir)
	installPlugins(Config.Plugins)
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
	if IsContainer() {
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

		androidSdk := os.Getenv(androidSdkRoot)
		if androidSdk != "" && IsContainer() {
			writeFileIfNew(filepath.Join(ideaOptions, "project.default.xml"), androidProjectDefaultXml(androidSdk))
			corettoSdk := os.Getenv(qodanaCorettoSdk)
			if corettoSdk != "" {
				writeFileIfNew(filepath.Join(ideaOptions, "jdk.table.xml"), jdkTableXml(corettoSdk))
			}
		}
	}
}

// installPlugins runs plugin installer for every plugin id in qodana.yaml.
func installPlugins(plugins []Plugin) {
	for _, plugin := range plugins {
		log.Printf("Installing plugin %s", plugin.Id)
		if res := RunCmd("", QuoteForWindows(Prod.IdeScript), "installPlugins", plugin.Id); res > 0 {
			os.Exit(res)
		}
	}
}

// syncIdeaCache sync .idea/ content from cache and back.
func syncIdeaCache(from string, to string, overwrite bool) {
	opt := cp.Options{}
	if overwrite {
		opt.OnDirExists = func(src, dest string) cp.DirExistsAction {
			return cp.Merge
		}
	} else {
		opt.OnDirExists = func(src, dest string) cp.DirExistsAction {
			return cp.Untouchable
		}
	}
	src := filepath.Join(from, ".idea")
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return
	}
	dst := filepath.Join(to, ".idea")
	log.Printf("Sync IDE cache from: %s to: %s", src, dst)
	if err := cp.Copy(src, dst, opt); err != nil {
		log.Fatal(err)
	}
}

func getScriptSuffix() string {
	if IsContainer() {
		return ".sh"
	}
	switch runtime.GOOS {
	case "windows":
		return ".bat"
	case "darwin":
		return ""
	default:
		return ".sh"
	}
}
