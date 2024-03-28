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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform"
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

var (
	EapSuffix   = "-EAP"
	releaseVer  = "release"
	eapVer      = "eap"
	versionsMap = map[string]string{
		releaseVer: "2023.3",
		eapVer:     "2024.1",
	}
	Products = map[string]string{
		platform.QDJVM:  "IIU",
		platform.QDJVMC: "IIC",
		// QDAND: // don't use it right now
		// QDANDC: // don't use it right now
		platform.QDPHP: "PS",
		platform.QDJS:  "WS",
		platform.QDNET: "RD",
		platform.QDPY:  "PCP",
		platform.QDPYC: "PCC",
		platform.QDGO:  "GO",
		platform.QDRST: "RR",
	}
)

func downloadAndInstallIDE(opts *QodanaOptions, baseDir string, spinner *pterm.SpinnerPrinter) string {
	if opts.Ide == "" || opts.guessProduct() == "" {
		log.Fatalf("Product code is not defined or not supported, exiting")
	}
	var ideUrl string
	checkSumUrl := ""

	releaseDownloadInfo := getIde(opts.Ide)
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
		log.Debugf("IDE already installed to %s, skipping download", installDir)
		return installDir
	}

	downloadedIdePath := filepath.Join(baseDir, fileName)
	err := platform.DownloadFile(downloadedIdePath, ideUrl, spinner)
	if err != nil {
		log.Fatalf("Error while downloading IDE: %v", err)
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
	case ".zip":
		err = installIdeWindowsZip(downloadedIdePath, installDir)
	case ".exe":
		err = installIdeWindowsExe(downloadedIdePath, installDir)
	case ".gz":
		err = installIdeLinux(downloadedIdePath, installDir)
	case ".dmg":
		err = installIdeMacOS(downloadedIdePath, installDir)
	default:
		log.Fatalf("Unsupported file extension: %s", fileExt)
	}

	if err != nil {
		log.Fatalf("Error while unpacking: %v", err)
	}

	return installDir
}

//goland:noinspection GoBoolExpressions
func getIde(productCode string) *ReleaseDownloadInfo {
	originalCode := productCode
	dist := releaseVer
	if strings.HasSuffix(productCode, EapSuffix) {
		dist = eapVer
		productCode = strings.TrimSuffix(productCode, EapSuffix)
	}

	if _, ok := Products[productCode]; !ok {
		platform.ErrorMessage("Product code doesnt exist: ", originalCode)
		return nil
	}

	supportedCode := false
	for _, v := range platform.AllNativeCodes {
		if v == productCode {
			supportedCode = true
			break
		}
	}

	if !supportedCode {
		platform.ErrorMessage("Product code is not supported: ", originalCode)
		return nil
	}

	product, err := GetProductByCode(Products[productCode])
	if err != nil || product == nil {
		platform.ErrorMessage("Error while obtaining the product info")
		return nil
	}

	release := SelectLatestCompatibleRelease(product, dist)
	if release == nil {
		platform.ErrorMessage("Error while obtaining the release type: ", dist)
		return nil
	}

	var downloadType string
	switch runtime.GOOS {
	case "darwin":
		downloadType = "mac"
		if runtime.GOARCH == "arm64" {
			downloadType = "macM1"
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
		platform.ErrorMessage("Error while obtaining the release for platform type: ", downloadType)
		return nil
	}

	log.Debug(fmt.Sprintf("%s %s %s %s URL: %s", productCode, dist, *release.Version, downloadType, res.Link))
	return &res
}

// installIdeWindowsExe is used as a fallback, since it needs installation privileges and alters the registry
func installIdeWindowsExe(archivePath string, targetDir string) error {
	_, err := exec.Command(archivePath, "/S", fmt.Sprintf("/D=%s", platform.QuoteForWindows(targetDir))).Output()
	if err != nil {
		return fmt.Errorf("%s: %s", archivePath, err)
	}
	return nil
}

func installIdeWindowsZip(archivePath string, targetDir string) error {
	if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
	_, err := exec.Command("tar", "-xf", platform.QuoteForWindows(archivePath), "--strip-components", "2", "-C", platform.QuoteForWindows(targetDir)).Output()
	if err != nil {
		return fmt.Errorf("tar: %s", err)
	}
	return nil
}

func installIdeLinux(archivePath string, targetDir string) error {
	if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
	_, err := exec.Command("tar", "-xf", archivePath, "-C", targetDir, "--strip-components", "2").Output()
	if err != nil {
		return fmt.Errorf("tar: %s", err)
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
	err := platform.DownloadFile(checksumFile, checkSumUrl, nil)
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
