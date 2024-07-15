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
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform"
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

func (p *product) IdeBin() string {
	return filepath.Join(p.Home, "bin")
}

func (p *product) CustomPluginsPath() string {
	return filepath.Join(p.Home, "custom-plugins")
}

func (p *product) javaHome() string {
	return filepath.Join(p.Home, "jbr")
}

func (p *product) JbrJava() string {
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
	case idea:
		return "IDEA_VM_OPTIONS"
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
	case clion:
		return "CLION_VM_OPTIONS"
	default:
		log.Fatalf("Usupported base script name for vmoptions file: %s", p.BaseScriptName)
		return ""
	}
}

func (p *product) parentPrefix() string {
	switch p.Code {
	case platform.QDPHP:
		return "PhpStorm"
	case platform.QDJS:
		return "WebStorm"
	case platform.QDNET:
		return "Rider"
	case platform.QDPY:
		return "Python"
	case platform.QDPYC:
		return "PyCharmCore"
	case platform.QDGO:
		return "GoLand"
	case platform.QDRUBY:
		return "Ruby"
	case platform.QDRST:
		return "RustRover"
	case platform.QDCPP:
		return "CLion"
	default:
		return "Idea"
	}
}

func (p *product) IsCommunity() bool {
	if p.Code == "" {
		return true
	}
	if platform.Contains(platform.AllSupportedFreeCodes, p.Code) {
		return true
	}
	return false
}

func (p *product) getProductNameFromCode() string {
	return getProductNameFromCode(p.Code)
}

func getProductNameFromCode(code string) string {
	switch code {
	case platform.QDJVMC:
		return "Qodana Community for JVM"
	case platform.QDPYC:
		return "Qodana Community for Python"
	case platform.QDANDC:
		return "Qodana Community for Android"
	case platform.QDAND:
		return "Qodana for Android"
	case platform.QDJVM:
		return "Qodana for JVM"
	case platform.QDPHP:
		return "Qodana for PHP"
	case platform.QDJS:
		return "Qodana for JS"
	case platform.QDNET:
		return "Qodana for .NET"
	case platform.QDNETC:
		return "Qodana Community for .NET"
	case platform.QDCL:
		return "Qodana for C/C++"
	case platform.QDPY:
		return "Qodana for Python"
	case platform.QDGO:
		return "Qodana for Go"
	case platform.QDRST:
		return "Qodana for Rust"
	case platform.QDRUBY:
		return "Qodana for Ruby"
	case platform.QDCPP:
		return "Qodana for C/C++"
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
		platform.WarningMessage("Invalid version: ", err)
		return false
	}
	return number >= 233
}

func (p *product) is242orNewer() bool {
	number, err := strconv.Atoi(p.getVersionBranch())
	if err != nil {
		platform.WarningMessage("Invalid version: ", err)
		return false
	}
	return number >= 242
}

var Prod product

// guessProduct fills all product fields.
func guessProduct(opts *QodanaOptions) {
	Prod.Home = opts.Ide
	if //goland:noinspection GoBoolExpressions
	runtime.GOOS == "darwin" {
		contentsDir := filepath.Join(Prod.Home, "Contents")
		if _, err := os.Stat(contentsDir); err == nil {
			Prod.Home = contentsDir
		}
	}
	if Prod.Home == "" {
		if home, ok := os.LookupEnv(platform.QodanaDistEnv); ok {
			Prod.Home = home
		} else if platform.IsContainer() {
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
			platform.WarningMessage(
				"Supported IDE not found in %s, you can declare the path to IDE home via %s variable",
				Prod.Home,
				platform.QodanaDistEnv,
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

	treatAsRelease := os.Getenv(platform.QodanaTreatAsRelease)
	if productInfo := readIdeProductInfo(Prod.Home); productInfo != nil {
		if v, ok := productInfo["version"]; ok {
			Prod.Version = v.(string)
		} else {
			Prod.Version = platform.Version
		}

		if v, ok := productInfo["buildNumber"]; ok {
			Prod.Build = v.(string)
		} else {
			Prod.Build = platform.Version
		}

		if v, ok := productInfo["productCode"]; ok {
			Prod.Code = toQodanaCode(v.(string))
			Prod.Name = Prod.getProductNameFromCode()
		} else {
			Prod.Code = scriptToProductCode(Prod.BaseScriptName)
		}

		if v, ok := productInfo["versionSuffix"]; ok {
			Prod.EAP = strings.HasPrefix(v.(string), "EAP")
		} else {
			Prod.EAP = false
		}
		if treatAsRelease == "true" {
			Prod.EAP = true
		}
	}

	log.Debug(Prod)
	platform.SetEnv(platform.QodanaDistEnv, Prod.Home)
}
