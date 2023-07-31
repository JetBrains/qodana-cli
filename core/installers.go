package core

import (
	"fmt"
	cp "github.com/otiai10/copy"
	log "github.com/sirupsen/logrus"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	EapSuffix = "-EAP"
)

func downloadAndInstallIDE(ide string, baseDir string) string {
	var url string
	if strings.HasPrefix(ide, "https://") || strings.HasPrefix(ide, "http://") {
		url = ide
	} else {
		url = getIde(ide)
		if url == "" {
			log.Fatalf("Error while obtaining the URL for the supplied IDE, exiting")
		}
	}

	fileName := filepath.Base(url)
	fileExt := filepath.Ext(fileName)
	installDir := filepath.Join(baseDir, strings.TrimSuffix(fileName, fileExt))
	if _, err := os.Stat(installDir); err == nil {
		log.Debugf("IDE already installed to %s, skipping download", installDir)
		return installDir
	}

	filePath := filepath.Join(baseDir, fileName)

	err := downloadFile(filePath, url)
	if err != nil {
		log.Fatalf("Error while downloading: %v", err)
	}

	switch fileExt {
	case ".zip":
		err = installIdeWindowsZip(filePath, installDir)
	case ".exe":
		err = installIdeWindowsExe(filePath, installDir)
	case ".gz":
		err = installIdeLinux(filePath, installDir)
	case ".dmg":
		err = installIdeMacOS(filePath, installDir)
	default:
		log.Fatalf("Unsupported file extension: %s", fileExt)
	}

	if err != nil {
		log.Fatalf("Error while unpacking: %v", err)
	}

	err = os.Remove(filePath)
	if err != nil {
		log.Warning("Error while removing temporary file: " + err.Error())
	}

	return installDir
}

//goland:noinspection GoBoolExpressions
func getIde(productCode string) string {
	products := map[string]string{
		QDJVM:  "IIU",
		QDJVMC: "IIC",
		QDAND:  "IIC",
		QDPHP:  "PS",
		QDJS:   "WS",
		QDNET:  "RD",
		QDPY:   "PCP",
		QDPYC:  "PCC",
		QDGO:   "GO",
	}

	originalCode := productCode
	dist := "release"
	if strings.HasSuffix(productCode, EapSuffix) {
		dist = "eap"
		productCode = strings.TrimSuffix(productCode, EapSuffix)
	}

	if _, ok := products[productCode]; !ok {
		ErrorMessage("Product code doesnt exist: ", originalCode)
		return ""
	}

	product, err := GetProductByCode(products[productCode])
	if err != nil || product == nil {
		ErrorMessage("Error while obtaining the product info")
		return ""
	}

	release := SelectLatestRelease(product, dist)
	if release == nil {
		ErrorMessage("Error while obtaining the release type: ", dist)
		return ""
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

	url, ok := (*release.Downloads)[downloadType]
	if !ok {
		ErrorMessage("Error while obtaining the release for platform type: ", downloadType)
		return ""
	}

	res := url.Link
	log.Debug(fmt.Sprintf("%s %s %s %s URL: %s", productCode, dist, *release.Version, downloadType, url.Link))
	return res
}

// installIdeWindowsExe is used as a fallback, since it needs installation privileges and alters the registry
func installIdeWindowsExe(archivePath string, targetDir string) error {
	_, err := exec.Command(archivePath, "/S", fmt.Sprintf("/D=%s", quoteForWindows(targetDir))).Output()
	if err != nil {
		return fmt.Errorf("%s: %s", archivePath, err)
	}
	return nil
}

func installIdeWindowsZip(archivePath string, targetDir string) error {
	if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
	_, err := exec.Command("tar", "-xf", quoteForWindows(archivePath), "-C", quoteForWindows(targetDir)).Output()
	if err != nil {
		return fmt.Errorf("tar: %s", err)
	}
	return nil
}

func installIdeLinux(archivePath string, targetDir string) error {
	if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
		log.Fatal("couldn't create a directory ", err.Error())
	}
	_, err := exec.Command("tar", "-xf", archivePath, "-C", targetDir, "--strip-components", "1").Output()
	if err != nil {
		return fmt.Errorf("tar: %s", err)
	}
	return nil
}

func installIdeMacOS(archivePath string, targetDir string) error {
	rand.Seed(time.Now().UnixNano())
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
