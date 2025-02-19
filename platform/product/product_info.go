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

package product

import (
	"encoding/json"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform/msg"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdenv"
	"github.com/JetBrains/qodana-cli/v2024/platform/utils"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

type Product struct {
	Name           string
	IdeCode        string
	Code           string
	Version        string
	BaseScriptName string
	IdeScript      string
	Build          string
	Home           string
	IsEap          bool
}

var ( // base script name
	Idea      = "idea"
	PhpStorm  = "phpstorm"
	WebStorm  = "webstorm"
	Rider     = "rider"
	PyCharm   = "pycharm"
	RubyMine  = "rubymine"
	GoLand    = "goland"
	RustRover = "rustrover"
	Clion     = "clion"
)

var supportedIdes = [...]string{
	Idea,
	PhpStorm,
	WebStorm,
	Rider,
	PyCharm,
	RubyMine,
	GoLand,
	RustRover,
	Clion,
}

func (p Product) IdeBin() string {
	return ideBin(p.Home)
}

func ideBin(home string) string {
	return filepath.Join(home, "bin")
}

func (p Product) CustomPluginsPath() string {
	return filepath.Join(p.Home, "custom-plugins")
}

func (p Product) javaHome() string {
	return filepath.Join(p.Home, "jbr")
}

func (p Product) JbrJava() string {
	if p.Home != "" {
		switch runtime.GOOS {
		case "darwin":
			return filepath.Join(p.javaHome(), "Contents", "Home", "bin", "java")
		case "windows":
			return filepath.Join(p.javaHome(), "bin", "java.exe")
		default:
			return filepath.Join(p.javaHome(), "bin", "java")
		}
	} else if utils.IsInstalled("java") {
		return "java"
	}
	log.Warn("Java is not installed")
	return ""
}

func (p Product) VmOptionsEnv() string {
	switch p.BaseScriptName {
	case Idea:
		return "IDEA_VM_OPTIONS"
	case PhpStorm:
		return "PHPSTORM_VM_OPTIONS"
	case WebStorm:
		return "WEBIDE_VM_OPTIONS"
	case Rider:
		return "RIDER_VM_OPTIONS"
	case PyCharm:
		return "PYCHARM_VM_OPTIONS"
	case RubyMine:
		return "RUBYMINE_VM_OPTIONS"
	case GoLand:
		return "GOLAND_VM_OPTIONS"
	case RustRover:
		return "RUSTROVER_VM_OPTIONS"
	case Clion:
		return "CLION_VM_OPTIONS"
	default:
		log.Fatalf("Usupported base script name for vmoptions file: %s", p.BaseScriptName)
		return ""
	}
}

func (p Product) ParentPrefix() string {
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
	case QDCPP:
		return "CLion"
	default:
		return "Idea"
	}
}

func (p Product) IsCommunity() bool {
	if p.Code == "" {
		return true
	}
	if utils.Contains(AllSupportedFreeCodes, p.Code) {
		return true
	}
	return false
}

func (p Product) GetProductNameFromCode() string {
	return GetProductNameFromCode(p.Code)
}

func GetProductNameFromCode(code string) string {
	switch code {
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
	case QDNETC:
		return "Qodana Community for .NET"
	case QDCL:
		return "Qodana for C/C++"
	case QDPY:
		return "Qodana for Python"
	case QDGO:
		return "Qodana for Go"
	case QDRST:
		return "Qodana for Rust"
	case QDRUBY:
		return "Qodana for Ruby"
	case QDCPP:
		return "Qodana for C/C++"
	default:
		return "Qodana"
	}
}

// GetVersionBranch returns the version branch of the current Product, e.g. 2020.3 -> 203, 2021.1 -> 211, 2022.3 -> 223.
func (p Product) GetVersionBranch() string {
	versions := strings.Split(p.Version, ".")
	if len(versions) < 2 {
		return "master"
	}
	return fmt.Sprintf("%s%s", versions[0][2:], versions[1])
}

// Is233orNewer returns true if the current Product is 233 or newer.
func (p Product) Is233orNewer() bool {
	return p.isNotOlderThan(233)
}

func (p Product) Is242orNewer() bool {
	return p.isNotOlderThan(242)
}

func (p Product) Is251orNewer() bool {
	return p.isNotOlderThan(251)
}

func (p Product) isNotOlderThan(version int) bool {
	number, err := strconv.Atoi(p.GetVersionBranch())
	if err != nil {
		msg.WarningMessage("Invalid version: ", err)
		return false
	}
	return number >= version
}

func (p Product) isRuby() bool {
	return p.Code == QDRUBY
}

type Launch struct {
	CustomCommands []struct {
		Commands               []string
		AdditionalJvmArguments []string
	} `json:"customCommands"`
}

type InfoJson struct {
	Version       string   `json:"version"`
	BuildNumber   string   `json:"buildNumber"`
	ProductCode   string   `json:"productCode"`
	VersionSuffix string   `json:"versionSuffix"`
	Launch        []Launch `json:"launch"`
}

func GuessProduct(idePath string) Product {
	homePath := idePath
	if //goland:noinspection GoBoolExpressions
	runtime.GOOS == "darwin" {
		contentsDir := filepath.Join(homePath, "Contents")
		if _, err := os.Stat(contentsDir); err == nil {
			homePath = contentsDir
		}
	}
	if homePath == "" {
		if home, ok := os.LookupEnv(qdenv.QodanaDistEnv); ok {
			homePath = home
		} else if qdenv.IsContainer() {
			homePath = "/opt/idea"
		} else { // guess from the executable location
			ex, err := os.Executable()
			if err != nil {
				log.Fatal(err)
			}
			homePath = filepath.Dir(filepath.Dir(ex))
		}
	}

	ideBinPath := ideBin(homePath)

	var baseScriptName string
	if //goland:noinspection GoBoolExpressions
	runtime.GOOS == "darwin" {
		baseScriptName = findIde(filepath.Join(homePath, "MacOS"))
	} else {
		baseScriptName = findIde(ideBinPath)
	}
	if baseScriptName == "" {
		msg.WarningMessage(
			"Supported IDE not found in %s, you can declare the path to IDE home via %s variable",
			homePath,
			qdenv.QodanaDistEnv,
		)
		log.Fatal("IDE to run is not found")
	}

	var ideScript string
	if //goland:noinspection ALL
	runtime.GOOS == "darwin" {
		ideScript = filepath.Join(homePath, "MacOS", baseScriptName)
	} else {
		ideScript = filepath.Join(ideBinPath, fmt.Sprintf("%s%s", baseScriptName, getScriptSuffix()))
	}
	productInfo, err := ReadIdeProductInfo(homePath)
	if err != nil {
		log.Fatalf("Can't read product-info.json: %v ", err)
	}

	version := productInfo.Version
	ideCode := productInfo.ProductCode
	code := toQodanaCode(ideCode)
	name := GetProductNameFromCode(code)
	build := productInfo.BuildNumber
	eap := isEap(*productInfo)

	prod := Product{
		Name:           name,
		IdeCode:        ideCode,
		Code:           code,
		Version:        version,
		BaseScriptName: baseScriptName,
		IdeScript:      ideScript,
		Build:          build,
		Home:           homePath,
		IsEap:          eap,
	}

	log.Debug(prod)
	qdenv.SetEnv(qdenv.QodanaDistEnv, prod.Home)
	if prod.isRuby() {
		qdenv.UnsetRubyVariables()
	}
	return prod
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
	case "CL":
		return QDCPP
	default:
		return "QD"
	}
}

func isEap(info InfoJson) bool {
	treatAsRelease := os.Getenv(qdenv.QodanaTreatAsRelease)
	if treatAsRelease == "true" {
		return true
	}

	for _, launch := range info.Launch {
		for _, command := range launch.CustomCommands {
			for _, cmd := range command.Commands {
				if cmd == "qodana" {
					for _, arg := range command.AdditionalJvmArguments {
						if arg == "-Dqodana.eap=true" {
							return true
						}
					}
				}
			}
		}
	}

	return false
}

// ReadIdeProductInfo returns IDE info from the given path.
func ReadIdeProductInfo(ideDir string) (*InfoJson, error) {
	if //goland:noinspection ALL
	runtime.GOOS == "darwin" {
		ideDir = filepath.Join(ideDir, "Resources")
	}
	productInfo := filepath.Join(ideDir, "product-info.json")
	if _, err := os.Stat(productInfo); err != nil {
		return nil, err
	}
	productInfoFile, err := os.ReadFile(productInfo)
	if err != nil {
		return nil, err
	}
	var productInfoJson InfoJson
	err = json.Unmarshal(productInfoFile, &productInfoJson)
	if err != nil {
		return nil, err
	}
	return &productInfoJson, nil
}

func findIde(dir string) string {
	for _, element := range supportedIdes {
		if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("%s%s", element, getScriptSuffix()))); err == nil {
			return element
		}
	}
	return ""
}

//goland:noinspection GoBoolExpressions
func getScriptSuffix() string {
	if runtime.GOOS == "windows" {
		return "64.exe"
	}
	return ""
}
