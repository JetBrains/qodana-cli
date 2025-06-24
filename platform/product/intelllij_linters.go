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
	FeedProductCode     string
	VmOptionsEnv        string
	ScriptName          string
}

var (
	JvmLinterProperties = IntellijLinterProperties{
		Linter:              JvmLinter,
		ProductInfoJsonCode: "IU",
		FeedProductCode:     "IIU",
		VmOptionsEnv:        "IDEA_VM_OPTIONS",
		ScriptName:          "idea",
	}

	JvmCommunityLinterProperties = IntellijLinterProperties{
		Linter:              JvmCommunityLinter,
		ProductInfoJsonCode: "IC",
		FeedProductCode:     "IIC",
		VmOptionsEnv:        "IDEA_VM_OPTIONS",
		ScriptName:          "idea",
	}

	AndroidLinterProperties = IntellijLinterProperties{
		Linter:              AndroidLinter,
		ProductInfoJsonCode: "IU",
		FeedProductCode:     "",
		VmOptionsEnv:        "IDEA_VM_OPTIONS",
		ScriptName:          "idea",
	}

	AndroidCommunityLinterProperties = IntellijLinterProperties{
		Linter:              AndroidCommunityLinter,
		ProductInfoJsonCode: "IC",
		FeedProductCode:     "",
		VmOptionsEnv:        "IDEA_VM_OPTIONS",
		ScriptName:          "idea",
	}

	PhpLinterProperties = IntellijLinterProperties{
		Linter:              PhpLinter,
		ProductInfoJsonCode: "PS",
		FeedProductCode:     "PS",
		VmOptionsEnv:        "PHPSTORM_VM_OPTIONS",
		ScriptName:          "phpstorm",
	}

	PythonLinterProperties = IntellijLinterProperties{
		Linter:              PythonLinter,
		ProductInfoJsonCode: "PY",
		FeedProductCode:     "PCP",
		VmOptionsEnv:        "PYCHARM_VM_OPTIONS",
		ScriptName:          "pycharm",
	}

	PythonLinterCommunityProperties = IntellijLinterProperties{
		Linter:              PythonCommunityLinter,
		ProductInfoJsonCode: "PC",
		FeedProductCode:     "PCC",
		VmOptionsEnv:        "PYCHARM_VM_OPTIONS",
		ScriptName:          "pycharm",
	}

	JsLinterProperties = IntellijLinterProperties{
		Linter:              JsLinter,
		ProductInfoJsonCode: "WS",
		FeedProductCode:     "WS",
		VmOptionsEnv:        "WEBIDE_VM_OPTIONS",
		ScriptName:          "webstorm",
	}

	NetLinterProperties = IntellijLinterProperties{
		Linter:              DotNetLinter,
		ProductInfoJsonCode: "RD",
		FeedProductCode:     "RD",
		VmOptionsEnv:        "RIDER_VM_OPTIONS",
		ScriptName:          "rider",
	}

	RubyLinterProperties = IntellijLinterProperties{
		Linter:              RubyLinter,
		ProductInfoJsonCode: "RM",
		FeedProductCode:     "RM",
		VmOptionsEnv:        "RUBYMINE_VM_OPTIONS",
		ScriptName:          "rubymine",
	}

	CppLinterProperties = IntellijLinterProperties{
		Linter:              CppLinter,
		ProductInfoJsonCode: "CL",
		FeedProductCode:     "CL",
		VmOptionsEnv:        "CLION_VM_OPTIONS",
		ScriptName:          "clion",
	}

	GoLinterProperties = IntellijLinterProperties{
		Linter:              GoLinter,
		ProductInfoJsonCode: "GO",
		FeedProductCode:     "GO",
		VmOptionsEnv:        "GOLAND_VM_OPTIONS",
		ScriptName:          "goland",
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
