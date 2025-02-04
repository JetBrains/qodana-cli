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

package startup

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/cloud"
	"github.com/JetBrains/qodana-cli/v2024/core"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/JetBrains/qodana-cli/v2024/platform/tokenloader"
	"github.com/JetBrains/qodana-cli/v2024/preparehost/product"
	"github.com/JetBrains/qodana-cli/v2024/preparehost/startupargs"
	cp "github.com/otiai10/copy"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	m2    = ".m2"
	nuget = "nuget"
)

type PreparedHost struct {
	IdeDir      string
	QodanaToken string
	Prod        product.Product
}

// prepareHost gets the current user, creates the necessary folders for the analysis.
func PrepareHost(args startupargs.Args) PreparedHost {
	prod := product.Product{}
	token := args.QodanaToken
	ideDir := ""

	if args.IsClearCache {
		err := os.RemoveAll(args.CacheDir)
		if err != nil {
			log.Errorf("Could not clear local Qodana cache: %s", err)
		}
	}
	platform.WarnIfPrivateFeedDetected(args.Linter, args.ProjectDir)
	if platform.IsNugetConfigNeeded() {
		platform.PrepareNugetConfig(os.Getenv("HOME"))
	}
	if err := os.MkdirAll(args.CacheDir, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
	if err := os.MkdirAll(args.ResultsDir, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
	if args.Linter != "" {
		core.PrepareContainerEnvSettings()
	}
	if args.Ide != "" {
		if platform.Contains(platform.AllNativeCodes, strings.TrimSuffix(args.Ide, EapSuffix)) {
			platform.PrintProcess(func(spinner *pterm.SpinnerPrinter) {
				if spinner != nil {
					spinner.ShowTimer = false // We will update interactive spinner
				}
				ideDir = downloadAndInstallIDE(args.Ide, args.Linter, args.QodanaSystemDir, spinner)
				fixWindowsPlugins(ideDir)
			}, fmt.Sprintf("Downloading %s", args.Ide), fmt.Sprintf("downloading IDE distribution to %s", args.QodanaSystemDir))
		} else {
			val, exists := os.LookupEnv(platform.QodanaDistEnv)
			if !exists || val == "" {
				log.Fatalf("Product code %s is not supported. ", args.Ide)
			} else if args.Ide != val {
				log.Fatalf("--ide argument '%s' doesn't match env variable %s value '%s'", args.Ide, platform.QodanaDistEnv, val)
			}
			ideDir = val
		}
		prod, token = prepareLocalIdeSettingsAndGetQodanaCloudToken(args)
	}
	args.QodanaToken = token

	if tokenloader.IsCloudTokenRequired(args, prod.IsCommunity() || prod.IsEap) {
		token = tokenloader.ValidateToken(args, false)
	}

	result := PreparedHost{
		IdeDir:      ideDir,
		QodanaToken: token,
		Prod:        prod,
	}
	return result
}

func prepareLocalIdeSettingsAndGetQodanaCloudToken(args startupargs.Args) (product.Product, string) {
	prod := product.GuessProduct(args.Ide)

	platform.ExtractQodanaEnvironment(platform.SetEnv)
	isTokenRequired := tokenloader.IsCloudTokenRequired(args, prod.IsEap || prod.IsCommunity())
	token := tokenloader.LoadCloudToken(args, false, isTokenRequired, true)
	cloud.SetupLicenseToken(token)
	core.SetupLicenseAndProjectHash(prod, cloud.GetCloudApiEndpoints(), cloud.Token.Token)
	PrepareDirectories(
		prod,
		args.CacheDir,
		args.LogDir(),
		args.ConfDirPath(),
	)

	if platform.IsContainer() {
		err := SyncIdeaCache(args.CacheDir, args.ProjectDir, false)
		if err != nil {
			log.Warnf("failed to sync .idea directory: %v", err)
		}
		SyncConfigCache(prod, args.ConfDirPath(), args.CacheDir, true)
		CreateUser("/etc/passwd")
	}
	return prod, token
}

func PrepareDirectories(prod product.Product, cacheDir string, logDir string, confDir string) {
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
		if prod.BaseScriptName == product.Rider {
			nugetDir := filepath.Join(cacheDir, nuget)
			if err := os.Setenv("NUGET_PACKAGES", nugetDir); err != nil {
				log.Fatal(err)
			}
			directories = append(
				directories,
				nugetDir,
			)
		} else if prod.BaseScriptName == product.Idea {
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

	if prod.BaseScriptName == product.Idea {
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

	disabledPluginsPathSrc := filepath.Join(prod.Home, "disabled_plugins.txt")
	disabledPluginsPathDst := filepath.Join(confDir, "disabled_plugins.txt")
	if _, err := os.Stat(disabledPluginsPathSrc); err == nil {
		if err := cp.Copy(disabledPluginsPathSrc, disabledPluginsPathDst); err != nil {
			log.Fatal(err)
		}
	}
}

func SyncConfigCache(prod product.Product, confDirPath string, cacheDir string, fromCache bool) {
	if prod.BaseScriptName == product.Idea {
		jdkTableFile := filepath.Join(confDirPath, "options", "jdk.table.xml")
		cacheFile := filepath.Join(cacheDir, "config", prod.GetVersionBranch(), "jdk.table.xml")
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
func SyncIdeaCache(from string, to string, overwrite bool) error {
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

func writeFileIfNew(filepath string, content string) {
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		if err := os.WriteFile(filepath, []byte(content), 0o755); err != nil {
			log.Fatal(err)
		}
	}
}

// CreateUser will make dynamic uid as a valid user `idea`, needed for gradle cache.
func CreateUser(fn string) {
	if //goland:noinspection ALL
	os.Getuid() == 0 {
		return
	}
	idea := fmt.Sprintf("idea:x:%d:%d:idea:/root:/bin/bash", os.Getuid(), os.Getgid())
	data, err := os.ReadFile(fn)
	if err != nil {
		log.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		if line == idea {
			return
		}
	}
	if err = os.WriteFile(fn, []byte(strings.Join(append(lines, idea), "\n")), 0o777); err != nil {
		log.Fatal(err)
	}
}

// fixWindowsPlugins quick-fix for Windows 241 distributions
func fixWindowsPlugins(ideDir string) {
	if runtime.GOOS == "windows" && strings.Contains(ideDir, "241") {
		pluginsClasspath := filepath.Join(ideDir, "plugins", "plugin-classpath.txt")
		if _, err := os.Stat(pluginsClasspath); err == nil {
			err = os.Remove(pluginsClasspath)
			if err != nil {
				log.Warnf("Failed to remove plugin-classpath.txt: %v", err)
			}
		}
	}
}
