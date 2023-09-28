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

func (p *product) IdeBin() string {
	return filepath.Join(p.Home, "bin")
}

func (p *product) javaHome() string {
	return filepath.Join(p.Home, "jbr")
}

func (p *product) JbrJava() string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(p.javaHome(), "Contents", "Home", "bin", "java")
	case "windows":
		return filepath.Join(p.javaHome(), "bin", "java.exe")
	default:
		return filepath.Join(p.javaHome(), "bin", "java")
	}
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

func (p *product) IsCommunity() bool {
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

var Prod product

// guessProduct fills all product fields.
func guessProduct(opts *QodanaOptions) {
	Prod.Home = opts.Ide
	if //goland:noinspection GoBoolExpressions
	runtime.GOOS == "darwin" {
		Prod.Home = filepath.Join(Prod.Home, "Contents")
	}
	if Prod.Home == "" {
		if home, ok := os.LookupEnv(QodanaDistEnv); ok {
			Prod.Home = home
		} else if IsContainer() {
			Prod.Home = "/opt/idea"
		} else { // guess from the executable location
			ex, err := os.Executable()
			if err != nil {
				log.Fatal(err)
			}
			Prod.Home = filepath.Dir(filepath.Dir(ex))
		}
	}

	if Prod.BaseScriptName == "" {
		if //goland:noinspection GoBoolExpressions
		runtime.GOOS == "darwin" {
			Prod.BaseScriptName = findIde(filepath.Join(Prod.Home, "MacOS"))
		} else {
			Prod.BaseScriptName = findIde(Prod.IdeBin())
		}
		if Prod.BaseScriptName == "" {
			WarningMessage(
				"Supported IDE not found in %s, you can declare the path to IDE home via %s variable",
				Prod.Home,
				QodanaDistEnv,
			)
			return
		}
	}

	if Prod.IdeScript == "" {
		if //goland:noinspection ALL
		runtime.GOOS == "darwin" {
			Prod.IdeScript = filepath.Join(Prod.Home, "MacOS", Prod.BaseScriptName)
		} else {
			Prod.IdeScript = filepath.Join(Prod.IdeBin(), fmt.Sprintf("%s%s", Prod.BaseScriptName, getScriptSuffix()))
		}
	}

	treatAsRelease := os.Getenv(QodanaTreatAsRelease)
	if _, err := os.Stat(filepath.Join(Prod.IdeBin(), qodanaAppInfoFilename)); err == nil && IsContainer() {
		appInfoContents := readAppInfoXml(Prod.Home)
		Prod.Version = appInfoContents.Version.Major + "." + appInfoContents.Version.Minor
		Prod.Build = strings.Split(appInfoContents.Build.Number, "-")[1]
		Prod.Code = strings.Split(appInfoContents.Build.Number, "-")[0]
		Prod.Name = appInfoContents.Names.Fullname
		Prod.EAP = appInfoContents.Version.Eap == "true" && !(treatAsRelease == "true")

	} else if productInfo := readIdeProductInfo(Prod.Home); productInfo != nil {
		if v, ok := productInfo["version"]; ok {
			Prod.Version = v.(string)
		} else {
			Prod.Version = version
		}

		if v, ok := productInfo["buildNumber"]; ok {
			Prod.Build = v.(string)
		} else {
			Prod.Build = version
		}

		if v, ok := productInfo["productCode"]; ok {
			Prod.Code = toQodanaCode(v.(string))
			Prod.Name = Prod.getProductNameFromCode()
		} else {
			Prod.Code = scriptToProductCode(Prod.BaseScriptName)
		}

		if v, ok := productInfo["versionSuffix"]; ok {
			Prod.EAP = v.(string) == "EAP"
		} else {
			Prod.EAP = false
		}
		if treatAsRelease == "true" {
			Prod.EAP = true
		}
	}

	if !IsContainer() {
		remove := fmt.Sprintf("-Didea.platform.prefix=%s", Prod.parentPrefix())
		Prod.IdeScript = patchIdeScript(Prod, remove, opts.ConfDirPath())
	}

	log.Debug(Prod)
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

func writeAppInfo(path string) {
	if _, err := os.Stat(path); err == nil && IsContainer() {
		return
	}
	log.Printf("Writing app info to %s", path)
	appInfoContents := []byte(appInfoXml(
		Prod.Version,
		Prod.EAP,
		Prod.Build,
		Prod.Code,
		Prod.Name,
	))
	err := os.WriteFile(path, appInfoContents, 0o777)
	if err != nil {
		log.Fatal(err)
	}
}
