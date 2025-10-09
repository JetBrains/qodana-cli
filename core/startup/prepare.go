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
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/JetBrains/qodana-cli/v2025/cloud"
	"github.com/JetBrains/qodana-cli/v2025/platform/commoncontext"
	"github.com/JetBrains/qodana-cli/v2025/platform/git"
	"github.com/JetBrains/qodana-cli/v2025/platform/msg"
	"github.com/JetBrains/qodana-cli/v2025/platform/nuget"
	"github.com/JetBrains/qodana-cli/v2025/platform/product"
	"github.com/JetBrains/qodana-cli/v2025/platform/qdcontainer"
	"github.com/JetBrains/qodana-cli/v2025/platform/qdenv"
	"github.com/JetBrains/qodana-cli/v2025/platform/tokenloader"
	cp "github.com/otiai10/copy"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
)

const (
	m2       = ".m2"
	nugetDir = "nuget"
)

type PreparedHost struct {
	IdeDir            string
	QodanaUploadToken string
	Prod              product.Product
}

// PrepareHost gets the current user, creates the necessary folders for the analysis.
func PrepareHost(commonCtx commoncontext.Context) PreparedHost {
	prod := product.Product{}
	cloudUploadToken := commonCtx.QodanaToken
	ideDir := ""

	if commonCtx.IsClearCache {
		err := os.RemoveAll(commonCtx.CacheDir)
		if err != nil {
			log.Errorf("Could not clear local Qodana cache: %s", err)
		}
	}
	nuget.WarnIfPrivateFeedDetected(commonCtx.Analyzer, commonCtx.ProjectDir)
	if nuget.IsNugetConfigNeeded() {
		nuget.PrepareNugetConfig(os.Getenv("HOME"))
	}
	if err := os.MkdirAll(commonCtx.CacheDir, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
	if err := os.MkdirAll(commonCtx.ResultsDir, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
	if err := os.MkdirAll(commonCtx.ReportDir, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}

	if commonCtx.Analyzer.DownloadDist() {
		linter := commonCtx.Analyzer.GetLinter()
		msg.PrintProcess(
			func(spinner *pterm.SpinnerPrinter) {
				if spinner != nil {
					spinner.ShowTimer = false // We will update interactive spinner
				}
				ideDir = downloadAndInstallIDE(commonCtx.Analyzer, commonCtx.QodanaSystemDir, spinner)
				fixWindowsPlugins(ideDir)
			},
			fmt.Sprintf("Downloading %s", linter.Name),
			fmt.Sprintf("downloading IDE distribution to %s", commonCtx.QodanaSystemDir),
		)
	}

	if commonCtx.Analyzer.IsContainer() {
		qdcontainer.PrepareContainerEnvSettings()
	} else {
		prod, cloudUploadToken = prepareLocalIdeSettingsAndGetQodanaCloudUploadToken(commonCtx, ideDir)
		// in case of container run the token passed directly (ref - core/container.go#getDockerOptions)
		prepareQodanaTokenForNative(cloudUploadToken)
	}

	if tokenloader.IsCloudTokenRequired(commonCtx) {
		cloudUploadToken = tokenloader.ValidateCloudToken(commonCtx, false)
	}
	checkVcsSameAsRepositoryRoot(commonCtx)

	result := PreparedHost{
		IdeDir:            ideDir,
		QodanaUploadToken: cloudUploadToken,
		Prod:              prod,
	}
	return result
}

func prepareLocalIdeSettingsAndGetQodanaCloudUploadToken(
	commonCtx commoncontext.Context,
	ideDir string,
) (product.Product, string) {
	prod := product.GuessProduct(ideDir, commonCtx.Analyzer)

	qdenv.ExtractQodanaEnvironment(qdenv.SetEnv)
	isTokenRequired := tokenloader.IsCloudTokenRequired(commonCtx)
	token := tokenloader.LoadCloudUploadToken(commonCtx, false, isTokenRequired, true)
	cloud.SetupLicenseToken(token)
	SetupLicenseAndProjectHash(prod, cloud.GetCloudApiEndpoints(), cloud.Token.Token)

	prepareDirectories(prod, commonCtx.CacheDir, commonCtx.LogDir(), commonCtx.ConfDirPath())

	if qdenv.IsContainer() {
		prepareContainerSpecificDirectories(prod, commonCtx.CacheDir, commonCtx.ConfDirPath())

		err := SyncIdeaCache(commonCtx.CacheDir, commonCtx.ProjectDir, false)
		if err != nil {
			log.Warnf("failed to sync .idea directory: %v", err)
		}
		SyncConfigCache(prod, commonCtx.ConfDirPath(), commonCtx.CacheDir, true)
		CreateUser("/etc/passwd")
	}

	prepareCustomPlugins(prod)
	return prod, token
}

func prepareQodanaTokenForNative(token string) {
	_, isSet := os.LookupEnv(qdenv.QodanaToken)
	if !isSet {
		err := os.Setenv(qdenv.QodanaToken, token)
		if err != nil {
			log.Fatal("Cannot set QODANA_TOKEN environment variable. The result may not be uploaded to the Qodana cloud.")
		}
	}
}

func prepareCustomPlugins(prod product.Product) {
	if runtime.GOOS == "darwin" && !prod.Analyzer.IsContainer() {
		if info := getIde(prod.Analyzer); info != nil {
			err := downloadCustomPlugins(info.Link, filepath.Dir(prod.CustomPluginsPath()), nil)
			if err != nil {
				log.Warning("Error while downloading custom plugins: " + err.Error())
			}
		}
	}
}

func prepareContainerSpecificDirectories(prod product.Product, cacheDir string, confDir string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	userPrefsDir := filepath.Join(homeDir, ".java", ".userPrefs")
	MakeDirAll(userPrefsDir)

	if prod.BaseScriptName == product.Rider {
		nugetDir := filepath.Join(cacheDir, nugetDir)
		if err := os.Setenv("NUGET_PACKAGES", nugetDir); err != nil {
			log.Fatal(err)
		}
		MakeDirAll(nugetDir)
	} else if prod.BaseScriptName == product.Idea {
		MakeDirAll(filepath.Join(cacheDir, m2))
		if err = os.Setenv("GRADLE_USER_HOME", filepath.Join(cacheDir, "gradle")); err != nil {
			log.Fatal(err)
		}
	}

	writeFileIfNew(filepath.Join(userPrefsDir, "prefs.xml"), userPrefsXml)

	ideaOptions := filepath.Join(confDir, "options")

	if prod.BaseScriptName == product.Idea {
		mavenRootDir := filepath.Join(homeDir, ".m2")
		if _, err = os.Stat(mavenRootDir); os.IsNotExist(err) {
			if err = os.MkdirAll(mavenRootDir, 0o755); err != nil {
				log.Fatal(err)
			}
		}

		writeFileIfNew(filepath.Join(mavenRootDir, "settings.xml"), mavenSettingsXml)
		writeFileIfNew(filepath.Join(ideaOptions, "path.macros.xml"), mavenPathMacroxXml)

		androidSdk := os.Getenv(qdenv.AndroidSdkRoot)
		if androidSdk != "" {
			writeFileIfNew(filepath.Join(ideaOptions, "project.default.xml"), androidProjectDefaultXml(androidSdk))
			corettoSdk := os.Getenv(qdenv.QodanaCorettoSdk)
			if corettoSdk != "" {
				writeFileIfNew(filepath.Join(ideaOptions, "jdk.table.xml"), jdkTableXml(corettoSdk))
			}
		}
	}
}

func prepareDirectories(prod product.Product, cacheDir string, logDir string, confDir string) {
	MakeDirAll(cacheDir)
	MakeDirAll(logDir)

	ideaOptions := filepath.Join(confDir, "options")
	MakeDirAll(ideaOptions)
	addKeepassIDEConfig(ideaOptions)
}

func MakeDirAll(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatal(err)
		}
	}
}

func addKeepassIDEConfig(ideaOptions string) {
	if //goland:noinspection ALL
	runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		writeFileIfNew(filepath.Join(ideaOptions, "security.xml"), securityXml)
	}
}

func SyncConfigCache(prod product.Product, confDirPath string, cacheDir string, fromCache bool) {
	isIdea := prod.BaseScriptName == product.Idea
	isPyCharm := prod.BaseScriptName == product.PyCharm
	if !(isIdea || isPyCharm) {
		return
	}

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

// SyncIdeaCache sync .idea/ content from cache and back.
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

func checkVcsSameAsRepositoryRoot(ctx commoncontext.Context) {
	if vcsRoot, err := git.Root(ctx.RepositoryRoot, ctx.LogDir()); err == nil {
		vcsRootAbs, err1 := filepath.Abs(vcsRoot)
		repositoryRootAbs, err2 := filepath.Abs(ctx.RepositoryRoot)
		if err1 != nil || err2 != nil {
			log.Warnf("Failed to resolve absolute paths for git root check: vcs=%v, proj=%v", err1, err2)
		} else if vcsRootAbs != repositoryRootAbs {
			log.Warnf(
				"The git root directory is different from the project root directory. This may lead to incorrect results. VCS root: %s, project root: %s",
				vcsRootAbs,
				repositoryRootAbs,
			)
		}
	}
}
