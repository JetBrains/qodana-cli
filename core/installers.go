package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	cp "github.com/otiai10/copy"
	log "github.com/sirupsen/logrus"
)

func downloadAndInstallIDE(productCode string) string {
	url := getIde(productCode)
	fileName := filepath.Base(url)
	fileExt := filepath.Ext(fileName)
	installDir := filepath.Join(getQodanaSystemDir(), strings.TrimSuffix(fileName, fileExt))
	if _, err := os.Stat(installDir); err == nil {
		log.Debugf("IDE already installed to %s, skipping download", installDir)
		return installDir
	}

	filePath := filepath.Join(getQodanaSystemDir(), fileName)

	err := downloadFile(filePath, url)
	if err != nil {
		log.Fatalf("Error while downloading: %v", err)
	}

	switch fileExt {
	case ".exe":
		err = installIdeWindows(filePath, installDir)
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
	const ideVersion = "2023.1.4"

	products := map[string]string{
		QDJVMC: "idea/ideaIC",
		QDPHP:  "webide/PhpStorm",
		QDJS:   "webstorm/WebStorm",
		QDNET:  "rider/JetBrains.Rider",
		QDPY:   "python/pycharm-professional",
		QDPYC:  "python/pycharm-community",
		QDGO:   "go/goland",
	}

	if _, ok := products[productCode]; !ok {
		products[productCode] = "idea/ideaIU"
	}

	var ext, arch string
	switch runtime.GOOS {
	case "darwin":
		ext = ".dmg"
	case "windows":
		ext = ".exe"
	default:
		ext = ".tar.gz"
	}
	if runtime.GOARCH == "arm64" {
		arch = "-aarch64"
	}

	res := fmt.Sprintf("https://download.jetbrains.com/%s-%s%s%s", products[productCode], ideVersion, arch, ext)
	log.Debug("IDE URL: " + res)
	return res
}

func installIdeWindows(archivePath string, targetDir string) error {
	_, err := exec.Command(archivePath, "/S", fmt.Sprintf("/D=%s", targetDir)).Output()
	if err != nil {
		return fmt.Errorf("%s: %s", archivePath, err)
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
	mountDir := "/Volumes/MyTempMount"
	_, err := exec.Command("hdiutil", "attach", "-nobrowse", "-mountpoint", mountDir, archivePath).Output()
	if err != nil {
		return fmt.Errorf("hdiutil: %s", err)
	}
	defer func(command *exec.Cmd) {
		err := command.Run()
		if err != nil {
			log.Fatal(err)
		}
	}(exec.Command("hdiutil", "detach", mountDir))
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
