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
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

type product struct {
	Name           string
	Code           string
	Version        string
	BaseScriptName string
	IdeScript      string
	Build          string
	Home           string
	EAP            bool
}

type appInfo struct {
	XMLName xml.Name       `xml:"component"`
	Version appInfoVersion `xml:"version"`
	Build   appInfoBuild   `xml:"build"`
	Names   appInfoNames   `xml:"names"`
}

type appInfoVersion struct {
	XMLName xml.Name `xml:"version"`
	Major   string   `xml:"major,attr"`
	Minor   string   `xml:"minor,attr"`
	Eap     string   `xml:"eap,attr"`
}

type appInfoBuild struct {
	XMLName xml.Name `xml:"build"`
	Number  string   `xml:"number,attr"`
	Date    string   `xml:"date,attr"`
}

type appInfoNames struct {
	XMLName  xml.Name `xml:"names"`
	Product  string   `xml:"product,attr"`
	Fullname string   `xml:"fullname,attr"`
}

//goland:noinspection GoUnnecessarilyExportedIdentifiers
func (p *product) IdeBin() string {
	return filepath.Join(p.Home, "bin")
}

func (p *product) javaHome() string {
	return filepath.Join(p.Home, "jbr")
}

func (p *product) jbrJava() string {
	if p.Home != "" {
		switch runtime.GOOS {
		case "darwin":
			return filepath.Join(p.javaHome(), "Contents", "Home", "bin", "java")
		case "windows":
			return filepath.Join(p.javaHome(), "bin", "java.exe")
		default:
			return filepath.Join(p.javaHome(), "bin", "java")
		}
	} else if isInstalled("java") {
		return "java"
	}
	log.Warn("Java is not installed")
	return ""
}

func (p *product) vmOptionsEnv() string {
	switch p.BaseScriptName {
	case phpStorm:
		return "PHPSTORM_VM_OPTIONS"
	case webStorm:
		return "WEBIDE_VM_OPTIONS"
	case rider:
		return "RIDER_VM_OPTIONS"
	case pyCharm:
		return "PYCHARM_VM_OPTIONS"
	case rubyMine:
		return "RUBYMINE_VM_OPTIONS"
	case goLand:
		return "GOLAND_VM_OPTIONS"
	case rustRover:
		return "RUSTROVER_VM_OPTIONS"
	default:
		return "IDEA_VM_OPTIONS"
	}
}

func (p *product) parentPrefix() string {
	switch p.Code {
	case QDPHP:
		return "PhpStorm"
	case QDJS:
		return "WebStorm"
	case QDNET:
		return "Rider"
	case QDPY:
		return "Python"
	case QDPYC:
		return "PyCharmCore"
	case QDGO:
		return "GoLand"
	case QDRUBY:
		return "Ruby"
	case QDRST:
		return "RustRover"
	default:
		return "Idea"
	}
}

func (p *product) isCommunity() bool {
	return p.Code == QDJVMC || p.Code == QDPYC || p.Code == ""
}

func (p *product) getProductNameFromCode() string {
	switch p.Code {
	case QDJVMC:
		return "Qodana Community for JVM"
	case QDPYC:
		return "Qodana Community for Python"
	case QDANDC:
		return "Qodana Community for Android"
	case QDAND:
		return "Qodana for Android"
	case QDJVM:
		return "Qodana for JVM"
	case QDPHP:
		return "Qodana for PHP"
	case QDJS:
		return "Qodana for JS"
	case QDNET:
		return "Qodana for .NET"
	case QDPY:
		return "Qodana for Python"
	case QDGO:
		return "Qodana for Go"
	case QDRST:
		return "Qodana for Rust"
	case QDRUBY:
		return "Qodana for Ruby"
	default:
		return "Qodana"
	}
}

// getVersionBranch returns the version branch of the current product, e.g. 2020.3 -> 203, 2021.1 -> 211, 2022.3 -> 223.
func (p *product) getVersionBranch() string {
	versions := strings.Split(p.Version, ".")
	if len(versions) < 2 {
		return "master"
	}
	return fmt.Sprintf("%s%s", versions[0][2:], versions[1])
}

// is233orNewer returns true if the current product is 233 or newer.
func (p *product) is233orNewer() bool {
	number, err := strconv.Atoi(p.getVersionBranch())
	if err != nil {
		WarningMessage("Invalid version: ", err)
		return false
	}
	return number >= 233
}

var prod product

// guessProduct fills all product fields.
func guessProduct(opts *QodanaOptions) {
	prod.Home = opts.Ide
	if //goland:noinspection GoBoolExpressions
	runtime.GOOS == "darwin" {
		prod.Home = filepath.Join(prod.Home, "Contents")
	}
	if prod.Home == "" {
		if home, ok := os.LookupEnv(QodanaDistEnv); ok {
			prod.Home = home
		} else if IsContainer() {
			prod.Home = "/opt/idea"
		} else { // guess from the executable location
			ex, err := os.Executable()
			if err != nil {
				log.Fatal(err)
			}
			prod.Home = filepath.Dir(filepath.Dir(ex))
		}
	}

	if prod.BaseScriptName == "" {
		if //goland:noinspection GoBoolExpressions
		runtime.GOOS == "darwin" {
			prod.BaseScriptName = findIde(filepath.Join(prod.Home, "MacOS"))
		} else {
			prod.BaseScriptName = findIde(prod.IdeBin())
		}
		if prod.BaseScriptName == "" {
			WarningMessage(
				"Supported IDE not found in %s, you can declare the path to IDE home via %s variable",
				prod.Home,
				QodanaDistEnv,
			)
			return
		}
	}

	if prod.IdeScript == "" {
		if //goland:noinspection ALL
		runtime.GOOS == "darwin" {
			prod.IdeScript = filepath.Join(prod.Home, "MacOS", prod.BaseScriptName)
		} else {
			prod.IdeScript = filepath.Join(prod.IdeBin(), fmt.Sprintf("%s%s", prod.BaseScriptName, getScriptSuffix()))
		}
	}

	treatAsRelease := os.Getenv(qodanaTreatAsRelease)
	if _, err := os.Stat(filepath.Join(prod.IdeBin(), qodanaAppInfoFilename)); err == nil && IsContainer() {
		appInfoContents := readAppInfoXml(prod.Home)
		prod.Version = appInfoContents.Version.Major + "." + appInfoContents.Version.Minor
		prod.Build = strings.Split(appInfoContents.Build.Number, "-")[1]
		prod.Code = strings.Split(appInfoContents.Build.Number, "-")[0]
		prod.Name = appInfoContents.Names.Fullname
		prod.EAP = appInfoContents.Version.Eap == "true" && !(treatAsRelease == "true")

	} else if productInfo := readIdeProductInfo(prod.Home); productInfo != nil {
		if v, ok := productInfo["version"]; ok {
			prod.Version = v.(string)
		} else {
			prod.Version = version
		}

		if v, ok := productInfo["buildNumber"]; ok {
			prod.Build = v.(string)
		} else {
			prod.Build = version
		}

		if v, ok := productInfo["productCode"]; ok {
			prod.Code = toQodanaCode(v.(string))
			prod.Name = prod.getProductNameFromCode()
		} else {
			prod.Code = scriptToProductCode(prod.BaseScriptName)
		}

		if v, ok := productInfo["versionSuffix"]; ok {
			prod.EAP = v.(string) == "EAP"
		} else {
			prod.EAP = false
		}
		if treatAsRelease == "true" {
			prod.EAP = true
		}
	}

	if !IsContainer() {
		remove := fmt.Sprintf("-Didea.platform.prefix=%s", prod.parentPrefix())
		prod.IdeScript = patchIdeScript(prod, remove, opts.ConfDirPath())
	}

	log.Debug(prod)
	setEnv(QodanaDistEnv, prod.Home)
}

// temporary solution to fix runs in the native mode
func patchIdeScript(product product, strToRemove string, confDirPath string) string {
	ext := filepath.Ext(product.IdeScript)
	newFilePath := filepath.Join(confDirPath, fmt.Sprintf("%s%s", product.BaseScriptName, ext))
	contentBytes, err := os.ReadFile(product.IdeScript)
	if err != nil {
		WarningMessage("Warning, can't read original script: %s (probably test mode)", err)
		return product.IdeScript
	}

	modifiedContent := strings.ReplaceAll(string(contentBytes), strToRemove, "")
	if //goland:noinspection GoBoolExpressions
	runtime.GOOS == "windows" {
		modifiedContent = strings.ReplaceAll(modifiedContent, "SET \"IDE_BIN_DIR=%~dp0\"", "SET \"IDE_BIN_DIR=%QODANA_DIST%\\bin\"")
	} else if //goland:noinspection GoBoolExpressions
	runtime.GOOS == "linux" {
		modifiedContent = strings.ReplaceAll(modifiedContent, "IDE_BIN_HOME=$(dirname \"$(realpath \"$0\")\")", "IDE_BIN_HOME=$QODANA_DIST/bin")
	} else {
		WarningMessage("Warning, unsupported platform: %s", runtime.GOOS)
		return product.IdeScript
	}
	if _, err := os.Stat(confDirPath); os.IsNotExist(err) {
		err := os.MkdirAll(confDirPath, os.ModePerm)
		if err != nil {
			log.Fatalf("failed to create directory: %v", err)
		}
	}

	err = os.WriteFile(newFilePath, []byte(modifiedContent), 0755)
	if err != nil {
		log.Fatal(err)
	}

	return newFilePath
}

func getDateNow() string {
	return time.Now().Format("200601021504")
}

func writeAppInfo(path string) {
	if _, err := os.Stat(path); err == nil && IsContainer() {
		return
	}
	log.Printf("Writing app info to %s", path)
	appInfoContents := []byte(appInfoXml(
		prod.Version,
		prod.EAP,
		getDateNow(),
		prod.Build,
		prod.Code,
		prod.Name,
	))
	err := os.WriteFile(path, appInfoContents, 0o777)
	if err != nil {
		log.Fatal(err)
	}
}
