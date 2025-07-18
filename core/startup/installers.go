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
	"fmt"
	"github.com/JetBrains/qodana-cli/v2025/platform"
	"github.com/JetBrains/qodana-cli/v2025/platform/msg"
	"github.com/JetBrains/qodana-cli/v2025/platform/product"
	"github.com/JetBrains/qodana-cli/v2025/platform/strutil"
	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
	cp "github.com/otiai10/copy"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
		if runtime.GOOS == "windows" {
			if dirs, err := filepath.Glob(filepath.Join(installDir, "*")); err == nil && len(dirs) == 1 {
				installDir = dirs[0]
			}
		} else if runtime.GOOS == "darwin" {
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

	if runtime.GOOS == "windows" {
		if dirs, err := filepath.Glob(filepath.Join(installDir, "*")); err == nil && len(dirs) == 1 {
			installDir = dirs[0]
		}
	} else if runtime.GOOS == "darwin" {
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

	feedProductCode := linterProperties.FeedProductCode
	prod, err := GetProductByCode(feedProductCode)
	if err != nil {
		msg.ErrorMessage("Error while obtaining the product info: " + err.Error())
		return nil
	}
	if prod == nil {
		msg.ErrorMessage("Product info is not found for code: ", feedProductCode)
		return nil
	}

	release := SelectLatestCompatibleRelease(prod, dist)
	if release == nil {
		msg.ErrorMessage("Error while obtaining the release type: ", dist)
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
			feedProductCode,
			*release.Version,
			dist,
		)
		return nil
	}

	log.Debug(fmt.Sprintf("%s %s %s %s URL: %s", feedProductCode, dist, *release.Version, downloadType, res.Link))
	return &res
}

// installIdeWindowsExe is used as a fallback, since it needs installation privileges and alters the registry
func installIdeWindowsExe(archivePath string, targetDir string) error {
	output, err := exec.Command(
		archivePath,
		"/S",
		fmt.Sprintf("/D=%s", strutil.QuoteForWindows(targetDir)),
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s. Output: %s", archivePath, err, string(output))
	}
	return nil
}

func installIdeFromZip(archivePath string, targetDir string) error {
	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("couldn't clean target directory: %w", err)
	}

	//temp dir is required cause tar fails to extract over symlink directories
	tempDir, err := os.MkdirTemp("", "qodana_linter")
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			log.Warningf("couldn't clean temp directory: %s", err.Error())
		}
	}()

	if err != nil {
		return fmt.Errorf("couldn't create a temporary directory %w", err)
	}

	output, err := exec.Command(
		"tar",
		"-xf",
		strutil.QuoteForWindows(archivePath),
		"-C",
		strutil.QuoteForWindows(tempDir),
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("extracting files error: %w. Output: %s", err, string(output))
	}

	err = os.Rename(tempDir, targetDir)
	if err != nil {
		return fmt.Errorf("error moving linter files from temp to target: %w", err)
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
	if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
	output, err := exec.Command(
		"tar",
		"-xf",
		archivePath,
		"-C",
		targetDir,
		"--strip-components",
		"1",
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar: %s. Output: %s", err, string(output))
	}
	return nil
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

func downloadCustomPlugins(ideUrl string, targetDir string, spinner *pterm.SpinnerPrinter) error {
	pluginsUrl := getPluginsURL(ideUrl)
	log.Debugf("Downloading custom plugins from %s to %s", pluginsUrl, targetDir)

	if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
		return fmt.Errorf("couldn't create a directory %s: %v", targetDir, err)
	}

	archivePath := filepath.Join(targetDir, "custom-plugins.zip")
	err := utils.DownloadFile(archivePath, pluginsUrl, getInternalAuth(), spinner)
	if err != nil {
		return fmt.Errorf("error while downloading plugins: %v", err)
	}

	_, err = exec.Command("tar", "-xf", archivePath, "-C", targetDir).Output()
	if err != nil {
		return fmt.Errorf("tar: %s", err)
	}

	disabledPluginsPath := filepath.Join(targetDir, "custom-plugins", "disabled_plugins.txt")
	err = cp.Copy(disabledPluginsPath, filepath.Join(targetDir, "disabled_plugins.txt"))
	if err != nil {
		return fmt.Errorf("error while copying plugins: %s", err)
	}

	return nil
}

func getPluginsURL(ideUrl string) string {
	pluginsUrl := strings.Replace(ideUrl, "-aarch64", "", 1)
	if strings.Contains(pluginsUrl, ".sit") {
		return strings.Replace(pluginsUrl, ".sit", "-custom-plugins.zip", 1)
	} else if strings.Contains(pluginsUrl, ".win.zip") {
		return strings.Replace(pluginsUrl, ".win.zip", "-custom-plugins.zip", 1)
	} else {
		return strings.Replace(pluginsUrl, ".tar.gz", "-custom-plugins.zip", 1)
	}
}
