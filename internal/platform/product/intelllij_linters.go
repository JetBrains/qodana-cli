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

import "fmt"

type IntellijLinterProperties struct {
	Linter
	ProductInfoJsonCode string
	QodanaLinterName    string
	VmOptionsEnv        string
	ScriptName          string
}

var (
	JvmLinterProperties = IntellijLinterProperties{
		Linter:              JvmLinter,
		ProductInfoJsonCode: "IU",
		QodanaLinterName:    "qodana-jvm",
		VmOptionsEnv:        "IDEA_VM_OPTIONS",
		ScriptName:          "idea",
	}

	JvmCommunityLinterProperties = IntellijLinterProperties{
		Linter:              JvmCommunityLinter,
		ProductInfoJsonCode: "IC",
		QodanaLinterName:    "qodana-jvm-community",
		VmOptionsEnv:        "IDEA_VM_OPTIONS",
		ScriptName:          "idea",
	}

	AndroidLinterProperties = IntellijLinterProperties{
		Linter:              AndroidLinter,
		ProductInfoJsonCode: "IU",
		QodanaLinterName:    "qodana-android",
		VmOptionsEnv:        "IDEA_VM_OPTIONS",
		ScriptName:          "idea",
	}

	AndroidCommunityLinterProperties = IntellijLinterProperties{
		Linter:              AndroidCommunityLinter,
		ProductInfoJsonCode: "IC",
		QodanaLinterName:    "qodana-android-community",
		VmOptionsEnv:        "IDEA_VM_OPTIONS",
		ScriptName:          "idea",
	}

	PhpLinterProperties = IntellijLinterProperties{
		Linter:              PhpLinter,
		ProductInfoJsonCode: "PS",
		QodanaLinterName:    "qodana-php",
		VmOptionsEnv:        "PHPSTORM_VM_OPTIONS",
		ScriptName:          "phpstorm",
	}

	PythonLinterProperties = IntellijLinterProperties{
		Linter:              PythonLinter,
		ProductInfoJsonCode: "PY",
		QodanaLinterName:    "qodana-python",
		VmOptionsEnv:        "PYCHARM_VM_OPTIONS",
		ScriptName:          "pycharm",
	}

	PythonLinterCommunityProperties = IntellijLinterProperties{
		Linter:              PythonCommunityLinter,
		ProductInfoJsonCode: "PC",
		QodanaLinterName:    "qodana-python-community",
		VmOptionsEnv:        "PYCHARM_VM_OPTIONS",
		ScriptName:          "pycharm",
	}

	JsLinterProperties = IntellijLinterProperties{
		Linter:              JsLinter,
		ProductInfoJsonCode: "WS",
		QodanaLinterName:    "qodana-js",
		VmOptionsEnv:        "WEBIDE_VM_OPTIONS",
		ScriptName:          "webstorm",
	}

	NetLinterProperties = IntellijLinterProperties{
		Linter:              DotNetLinter,
		ProductInfoJsonCode: "RD",
		QodanaLinterName:    "qodana-dotnet",
		VmOptionsEnv:        "RIDER_VM_OPTIONS",
		ScriptName:          "rider",
	}

	RubyLinterProperties = IntellijLinterProperties{
		Linter:              RubyLinter,
		ProductInfoJsonCode: "RM",
		QodanaLinterName:    "qodana-ruby",
		VmOptionsEnv:        "RUBYMINE_VM_OPTIONS",
		ScriptName:          "rubymine",
	}

	CppLinterProperties = IntellijLinterProperties{
		Linter:              CppLinter,
		ProductInfoJsonCode: "CL",
		QodanaLinterName:    "qodana-cpp",
		VmOptionsEnv:        "CLION_VM_OPTIONS",
		ScriptName:          "clion",
	}

	GoLinterProperties = IntellijLinterProperties{
		Linter:              GoLinter,
		ProductInfoJsonCode: "GO",
		QodanaLinterName:    "qodana-go",
		VmOptionsEnv:        "GOLAND_VM_OPTIONS",
		ScriptName:          "goland",
	}

	RustLinterProperties = IntellijLinterProperties{
		Linter:              RustLinter,
		ProductInfoJsonCode: "RR",
		QodanaLinterName:    "qodana-rust",
		VmOptionsEnv:        "RUSTROVER_VM_OPTIONS",
		ScriptName:          "rustrover",
	}

	AllLinterProperties = []IntellijLinterProperties{
		JvmLinterProperties,
		JvmCommunityLinterProperties,
		AndroidLinterProperties,
		AndroidCommunityLinterProperties,
		PhpLinterProperties,
		PythonLinterProperties,
		PythonLinterCommunityProperties,
		JsLinterProperties,
		NetLinterProperties,
		RubyLinterProperties,
		CppLinterProperties,
		GoLinterProperties,
		RustLinterProperties,
	}
)

func FindLinterProperties(linter Linter) *IntellijLinterProperties {
	for _, properties := range AllLinterProperties {
		if properties.Linter == linter {
			return &properties
		}
	}
	return nil
}

func FindLinterPropertiesByProductInfo(productInfoCode string) (*IntellijLinterProperties, error) {
	for _, properties := range AllLinterProperties {
		if properties.ProductInfoJsonCode == productInfoCode {
			return &properties, nil
		}
	}
	return nil, fmt.Errorf("linter for product code %s not found", productInfoCode)
}
