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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	fexec "github.com/JetBrains/qodana-cli/internal/foundation/exec"
	"github.com/JetBrains/qodana-cli/internal/foundation/flock"
	"github.com/JetBrains/qodana-cli/internal/platform"
	"github.com/JetBrains/qodana-cli/internal/platform/msg"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/utils"
	cp "github.com/otiai10/copy"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
)

func downloadAndInstallIDE(
	analyser product.Analyzer,
	baseDir string,
	spinner *pterm.SpinnerPrinter,
) string {
	var ideUrl string
	checkSumUrl := ""

	releaseDownloadInfo := getIde(analyser)
	if releaseDownloadInfo == nil {
		log.Fatalf("Error while obtaining the URL for the supplied IDE, exiting")
	} else {
		ideUrl = releaseDownloadInfo.Link
		checkSumUrl = releaseDownloadInfo.ChecksumLink
	}

	fileName := filepath.Base(ideUrl)
	fileExt := filepath.Ext(fileName)
	installDir := filepath.Join(baseDir, strings.TrimSuffix(fileName, fileExt))
	if _, err := os.Stat(installDir); err == nil {
		switch runtime.GOOS {
		case "windows":
			if dirs, err := filepath.Glob(filepath.Join(installDir, "*")); err == nil && len(dirs) == 1 {
				installDir = dirs[0]
			}
		case "darwin":
			if dirs, err := filepath.Glob(filepath.Join(installDir, "*.app")); err == nil && len(dirs) == 1 {
				installDir = filepath.Join(dirs[0], "Contents")
			}
		}
		log.Debugf("IDE already installed to %s, skipping download", installDir)
		return installDir
	}

	downloadedIdePath := filepath.Join(baseDir, fileName)
	err := utils.DownloadFile(downloadedIdePath, ideUrl, getInternalAuth(), spinner)
	if err != nil {
		log.Fatalf("Error while downloading linter: %v", err)
	}

	defer func(filePath string) {
		err = os.Remove(filePath)
		if err != nil {
			log.Warning("Error while removing temporary file: " + err.Error())
		}
	}(downloadedIdePath)

	if checkSumUrl != "" {
		checksumFilePath := filepath.Join(baseDir, strings.TrimSuffix(fileName, fileExt)+".sha256")
		verifySha256(checksumFilePath, checkSumUrl, downloadedIdePath)
	}

	switch fileExt {
	case ".sit":
		err = installIdeFromZip(downloadedIdePath, installDir)
	case ".zip":
		err = installIdeFromZip(downloadedIdePath, installDir)
	case ".exe":
		err = installIdeWindowsExe(downloadedIdePath, installDir)
	case ".gz":
		err = installIdeFromTar(downloadedIdePath, installDir)
	case ".dmg":
		err = installIdeMacOS(downloadedIdePath, installDir)
	default:
		log.Fatalf("Unsupported file extension: %s", fileExt)
	}

	if err != nil {
		log.Fatalf("Error while unpacking: %v", err)
	}

	switch runtime.GOOS {
	case "windows":
		if dirs, err := filepath.Glob(filepath.Join(installDir, "*")); err == nil && len(dirs) == 1 {
			installDir = dirs[0]
		}
	case "darwin":
		if dirs, err := filepath.Glob(filepath.Join(installDir, "*.app")); err == nil && len(dirs) == 1 {
			installDir = filepath.Join(dirs[0], "Contents")
		}
	}

	return installDir
}

//goland:noinspection GoBoolExpressions
func getIde(analyzer product.Analyzer) *ReleaseDownloadInfo {

	dist := product.ReleaseVer
	if analyzer.IsEAP() {
		dist = product.EapVer
	}
	name := analyzer.GetLinter().Name
	if !analyzer.GetLinter().SupportNative {
		msg.ErrorMessage("Native mode for linter %s is not supported", name)
		return nil
	}

	linterProperties := product.FindLinterProperties(analyzer.GetLinter())
	if linterProperties == nil {
		msg.ErrorMessage("Native mode for linter %s is not supported", name)
		return nil
	}

	qodanaLinterName := linterProperties.QodanaLinterName
	if qodanaLinterName == "" {
		msg.ErrorMessage("Native mode for linter %s is not supported (no feed available)", name)
		return nil
	}

	prod, err := getProductByLinterName(qodanaLinterName)
	if err != nil {
		msg.ErrorMessage("Error while obtaining the product info: %s", err)
		return nil
	}
	if prod == nil {
		msg.ErrorMessage("Feed file not found for linter '%s' (native mode not available yet)", qodanaLinterName)
		return nil
	}

	release := selectLatestCompatibleRelease(prod, dist)
	if release == nil {
		msg.ErrorMessage("Could not find a %s version for '%s'", dist, linterProperties.PresentableName)
		return nil
	}

	var downloadType string
	switch runtime.GOOS {
	case "darwin":
		downloadType = "macSit"
		_, ok := (*release.Downloads)[downloadType]
		if !ok {
			downloadType = "mac"
		}
		if runtime.GOARCH == "arm64" {
			downloadType = "macSitM1"
			_, ok := (*release.Downloads)[downloadType]
			if !ok {
				downloadType = "macM1"
			}
		}
	case "windows":
		downloadType = "windowsZip"
		_, ok := (*release.Downloads)[downloadType]
		if !ok {
			downloadType = "windows"
		}
		if runtime.GOARCH == "arm64" {
			downloadType = "windowsZipARM64"
			_, ok := (*release.Downloads)[downloadType]
			if !ok {
				downloadType = "windowsARM64"
			}
		}
	default:
		downloadType = "linux"
		if runtime.GOARCH == "arm64" {
			downloadType = "linuxARM64"
		}
	}

	res, ok := (*release.Downloads)[downloadType]
	if !ok {
		msg.ErrorMessage(
			"%s %s (%s) is not available or not supported for the current platform",
			qodanaLinterName,
			*release.Version,
			dist,
		)
		return nil
	}

	log.Debug(fmt.Sprintf("%s %s %s %s URL: %s", qodanaLinterName, dist, *release.Version, downloadType, res.Link))
	return &res
}

// installIdeWindowsExe is used as a fallback, since it needs installation privileges and alters the registry
func installIdeWindowsExe(archivePath string, targetDir string) error {
	stdout, stderr, _, err := fexec.ExecRedirectOutput(
		".",
		archivePath,
		"/S",
		fmt.Sprintf("/D=%s", targetDir),
	)
	if err != nil {
		return fmt.Errorf("%s: %s. Stdout: %s. Stderr: %s", archivePath, err, stdout, stderr)
	}
	return nil
}

func extractArchive(archivePath string, targetDir string, stripComponents int) error {
	targetBasename := filepath.Base(targetDir)
	if targetBasename == "." || targetBasename == "/" {
		// filepath.Base returns either "." or "/" when the path has no basename.
		return fmt.Errorf("invalid target directory")
	}

	tempDir, err := os.MkdirTemp("", fmt.Sprintf("%s-*.partial", targetBasename))
	if err != nil {
		return fmt.Errorf("failed to create temporary directory for extraction: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			log.Warningf("failed to remove temporary directory: %s", err.Error())
		}
	}()

	tarExe, err := exec.LookPath("tar")
	if err != nil {
		return fmt.Errorf("could not find 'tar': %w", err)
	}

	tarArgv := []string{"-xf", archivePath, "-C", tempDir}
	if stripComponents > 0 {
		tarArgv = append(tarArgv, "--strip-components", strconv.Itoa(stripComponents))
	}

	stdout, stderr, _, err := fexec.ExecRedirectOutput(".", tarExe, tarArgv...)
	if err != nil {
		return fmt.Errorf("failed to extract: %w. Stdout: %s. Stderr: %s", err, stdout, stderr)
	}

	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("failed to remove existing target directory: %w", err)
	}
	err = os.Rename(tempDir, targetDir)
	if err != nil { // Trying to fallback if rename failed https://youtrack.jetbrains.com/issue/QD-12252
		log.Warningf("Error moving linter files from temp to target: %s. Trying to copy instead.", err)
		err = cp.Copy(tempDir, targetDir)
		if err != nil {
			return fmt.Errorf("error copying linter files from temp to target: %w", err)
		}
	}

	return nil
}

func installIdeFromZip(archivePath string, targetDir string) error {
	err := extractArchive(archivePath, targetDir, 0)
	if err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		err = platform.ChangePermissionsRecursivelyUnix(targetDir, 0755)
		if err != nil {
			return fmt.Errorf("error moving linter files from temp to target: %w", err)
		}
	}

	return nil
}

func installIdeFromTar(archivePath string, targetDir string) error {
	return extractArchive(archivePath, targetDir, 1)
}

func installIdeMacOS(archivePath string, targetDir string) error {
	mountDir := fmt.Sprintf("/Volumes/MyTempMount%d", rand.Intn(10000))
	_, err := exec.Command("hdiutil", "attach", "-nobrowse", "-mountpoint", mountDir, archivePath).Output()
	if err != nil {
		return fmt.Errorf("hdiutil attach: %s", err)
	}
	defer func(command *exec.Cmd) {
		err := command.Run()
		if err != nil {
			log.Fatal(fmt.Errorf("hdiutil eject: %s", err))
		}
	}(exec.Command("hdiutil", "eject", mountDir, "-force"))
	matches, err := filepath.Glob(mountDir + "/*.app")
	if err != nil {
		return fmt.Errorf("filepath.Glob: %s", err)
	}
	if len(matches) == 0 {
		return fmt.Errorf("no .app found in dmg")
	}

	err = cp.Copy(matches[0], targetDir)
	if err != nil {
		return fmt.Errorf("cp.Copy: %s", err)
	}

	return nil
}

func verifySha256(checksumFile string, checkSumUrl string, filePath string) {
	err := utils.DownloadFile(checksumFile, checkSumUrl, getInternalAuth(), nil)
	if err != nil {
		log.Fatalf("Error while downloading checksum for IDE: %v", err)
	}

	defer func(filePath string) {
		err = os.Remove(filePath)
		if err != nil {
			log.Warning("Error while removing temporary file: " + err.Error())
		}
	}(checksumFile)

	checksum, err := os.ReadFile(checksumFile)
	if err != nil {
		log.Fatalf("Error occurred during reading checksum file: %v", err)
	}

	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("Error while opening IDE archive: %v", err)
	}

	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Fatalf("Error while closing IDE archive: %v", err)
		}
	}(file)

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		log.Fatalf("Error while computing checksum of IDE archive: %v", err)
	}

	actual := hex.EncodeToString(h.Sum(nil))
	expected := strings.SplitN(string(checksum), " ", 2)[0]
	if actual != expected {
		err = os.Remove(filePath)
		if err != nil {
			log.Warning("Error while removing temporary file: " + err.Error())
		}
		log.Fatalf("Checksums doesn't match. Expected: %s, Actual: %s", expected, actual)
	}
	log.Info("Checksum of downloaded IDE was verified")
}

// customPluginsSourceFile stores the plugins URL used to populate the cache.
// Custom plugins live under <code>/<version-branch>/, so the URL (which includes
// the build number) is what invalidates the cache across patch releases.
const customPluginsSourceFile = "custom-plugins.source"

// customPluginsLockFile coordinates concurrent downloads into the same targetDir.
const customPluginsLockFile = "custom-plugins.lock"

func downloadCustomPlugins(ideUrl string, targetDir string, spinner *pterm.SpinnerPrinter) error {
	pluginsUrl := getPluginsURL(ideUrl)
	if isCustomPluginsCacheValid(targetDir, pluginsUrl) {
		log.Debugf("Custom plugins already downloaded from %s to %s, skipping download", pluginsUrl, targetDir)
		return nil
	}

	// Exclusive lock so concurrent runs for the same version-branch wait instead of
	// racing on staging/install. Re-check the cache after acquiring — another process
	// may have finished the download while we waited.
	lockPath := filepath.Join(targetDir, customPluginsLockFile)
	var downloadErr error
	if err := flock.With(lockPath, func() {
		if isCustomPluginsCacheValid(targetDir, pluginsUrl) {
			log.Debugf("Custom plugins already downloaded from %s to %s (updated by another process), skipping download", pluginsUrl, targetDir)
			return
		}
		downloadErr = downloadAndInstallCustomPlugins(pluginsUrl, targetDir, spinner)
	}); err != nil {
		return failedCustomPluginsUpdate(pluginsUrl, hasCustomPluginsInstall(targetDir), fmt.Errorf("couldn't acquire custom plugins lock: %w", err))
	}
	return downloadErr
}

func downloadAndInstallCustomPlugins(pluginsUrl string, targetDir string, spinner *pterm.SpinnerPrinter) error {
	log.Debugf("Downloading custom plugins from %s to %s", pluginsUrl, targetDir)
	hadPrevious := hasCustomPluginsInstall(targetDir)

	if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
		return failedCustomPluginsUpdate(pluginsUrl, hadPrevious, fmt.Errorf("couldn't create a directory %s: %v", targetDir, err))
	}
	cleanupStaleCustomPluginsStaging(targetDir)

	// Stage into a temp dir first so a failed download/unpack keeps the previous cache intact.
	stagingDir, err := os.MkdirTemp(targetDir, "custom-plugins.*.partial")
	if err != nil {
		return failedCustomPluginsUpdate(pluginsUrl, hadPrevious, fmt.Errorf("couldn't create staging directory: %v", err))
	}
	defer func() {
		if removeErr := os.RemoveAll(stagingDir); removeErr != nil {
			log.Warning("Error while removing custom plugins staging directory: " + removeErr.Error())
		}
	}()

	archivePath := filepath.Join(stagingDir, "custom-plugins.zip")
	err = utils.DownloadFile(archivePath, pluginsUrl, getInternalAuth(), spinner)
	if err != nil {
		return failedCustomPluginsUpdate(pluginsUrl, hadPrevious, fmt.Errorf("error while downloading plugins: %v", err))
	}

	// fexec.Exec returns a nil error for normal non-zero process exits; only the
	// exit code signals tar failure (e.g. partial extract of a malformed archive).
	exitCode, err := fexec.Exec(".", "tar", "-xf", archivePath, "-C", stagingDir)
	if err != nil {
		return failedCustomPluginsUpdate(pluginsUrl, hadPrevious, fmt.Errorf("tar: %w", err))
	}
	if exitCode != 0 {
		return failedCustomPluginsUpdate(pluginsUrl, hadPrevious, fmt.Errorf("tar: exit code %d", exitCode))
	}

	stagingPlugins := filepath.Join(stagingDir, "custom-plugins")
	stagingDisabled := filepath.Join(stagingDir, "disabled_plugins.txt")
	err = cp.Copy(filepath.Join(stagingPlugins, "disabled_plugins.txt"), stagingDisabled)
	if err != nil {
		return failedCustomPluginsUpdate(pluginsUrl, hadPrevious, fmt.Errorf("error while copying plugins: %s", err))
	}

	if err := os.WriteFile(filepath.Join(stagingDir, customPluginsSourceFile), []byte(pluginsUrl), 0o644); err != nil {
		return failedCustomPluginsUpdate(pluginsUrl, hadPrevious, fmt.Errorf("error while writing custom plugins source marker: %v", err))
	}

	if err := installCustomPluginsFromStaging(targetDir, stagingDir); err != nil {
		return failedCustomPluginsUpdate(pluginsUrl, hadPrevious, err)
	}

	return nil
}

func hasCustomPluginsInstall(targetDir string) bool {
	info, err := os.Stat(filepath.Join(targetDir, "custom-plugins"))
	return err == nil && info.IsDir()
}

// failedCustomPluginsUpdate logs update failures. When a previous install exists it
// stays on disk (staging never replaced it, or install was rolled back), so the
// warning says we are keeping it.
func failedCustomPluginsUpdate(pluginsUrl string, hadPrevious bool, err error) error {
	if hadPrevious {
		log.Warningf("Failed to update custom plugins from %s, keeping previously cached plugins: %v", pluginsUrl, err)
		return err
	}
	log.Warningf("Failed to update custom plugins from %s: %v", pluginsUrl, err)
	return err
}

func cleanupStaleCustomPluginsStaging(targetDir string) {
	matches, err := filepath.Glob(filepath.Join(targetDir, "custom-plugins.*.partial"))
	if err != nil {
		return
	}
	for _, match := range matches {
		if err := os.RemoveAll(match); err != nil {
			log.Warningf("Failed to remove stale custom plugins staging directory %s: %s", match, err)
		}
	}
}

// installCustomPluginsFromStaging replaces the live custom-plugins tree only after
// staging is complete. The previous tree is moved aside first so a rename failure
// can roll back instead of leaving an empty cache. The source marker is written
// last; any removal failure aborts before that so the old URL marker is kept.
func installCustomPluginsFromStaging(targetDir string, stagingDir string) error {
	finalPlugins := filepath.Join(targetDir, "custom-plugins")
	stagingPlugins := filepath.Join(stagingDir, "custom-plugins")
	backupPlugins := filepath.Join(targetDir, "custom-plugins.old")

	if err := os.RemoveAll(backupPlugins); err != nil {
		return fmt.Errorf("failed to remove leftover custom plugins backup: %w", err)
	}

	hadPrevious := false
	if info, err := os.Stat(finalPlugins); err == nil && info.IsDir() {
		if err := os.Rename(finalPlugins, backupPlugins); err != nil {
			return fmt.Errorf("failed to move previous custom plugins aside: %w", err)
		}
		hadPrevious = true
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to stat custom plugins directory: %w", err)
	}

	if err := os.Rename(stagingPlugins, finalPlugins); err != nil {
		if hadPrevious {
			if rollbackErr := os.Rename(backupPlugins, finalPlugins); rollbackErr != nil {
				log.Warningf("Failed to roll back custom plugins after install error: %s", rollbackErr)
			}
		}
		return fmt.Errorf("failed to install custom plugins: %w", err)
	}

	if hadPrevious {
		if err := os.RemoveAll(backupPlugins); err != nil {
			// New tree is already in place; put the previous one back and leave the
			// URL marker untouched so the cache is not claimed as updated.
			if rbErr := os.Rename(finalPlugins, stagingPlugins); rbErr != nil {
				log.Warningf("Failed to undo custom plugins install after removal error: %s", rbErr)
			} else if rbErr := os.Rename(backupPlugins, finalPlugins); rbErr != nil {
				log.Warningf("Failed to roll back custom plugins after removal error: %s", rbErr)
			}
			return fmt.Errorf("failed to remove previous custom plugins: %w", err)
		}
	}

	if err := os.Rename(filepath.Join(stagingDir, "disabled_plugins.txt"), filepath.Join(targetDir, "disabled_plugins.txt")); err != nil {
		return fmt.Errorf("failed to install disabled_plugins.txt: %w", err)
	}
	if err := os.Rename(filepath.Join(stagingDir, customPluginsSourceFile), filepath.Join(targetDir, customPluginsSourceFile)); err != nil {
		return fmt.Errorf("failed to install custom plugins source marker: %w", err)
	}

	return nil
}

func isCustomPluginsCacheValid(targetDir string, pluginsUrl string) bool {
	if _, err := os.Stat(filepath.Join(targetDir, "disabled_plugins.txt")); err != nil {
		return false
	}
	pluginsDir := filepath.Join(targetDir, "custom-plugins")
	info, err := os.Stat(pluginsDir)
	if err != nil || !info.IsDir() {
		return false
	}
	entries, err := os.ReadDir(pluginsDir)
	if err != nil || len(entries) == 0 {
		return false
	}
	data, err := os.ReadFile(filepath.Join(targetDir, customPluginsSourceFile))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == pluginsUrl
}

func getPluginsURL(ideUrl string) string {
	pluginsUrl := strings.Replace(ideUrl, "-aarch64", "", 1)
	if strings.Contains(pluginsUrl, ".sit") {
		return strings.Replace(pluginsUrl, ".sit", "-custom-plugins.zip", 1)
	} else if strings.Contains(pluginsUrl, ".win.zip") {
		return strings.Replace(pluginsUrl, ".win.zip", "-custom-plugins.zip", 1)
	}
	return strings.Replace(pluginsUrl, ".tar.gz", "-custom-plugins.zip", 1)
}
