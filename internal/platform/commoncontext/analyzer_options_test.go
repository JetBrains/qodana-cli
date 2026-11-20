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

package commoncontext

import (
	"github.com/JetBrains/qodana-cli/internal/platform/product"
)

type TestCase struct {
	name             string
	ide              string
	linter           string
	image            string
	executionMode    ExecutionMode
	yamlWithinDocker string
	failure          bool
	expected         product.Analyzer
}

//goland:noinspection ALL
var optionsTests = []TestCase{
	{
		name:          "--ide passed",
		ide:           "QDNET",
		linter:        "",
		image:         "",
		executionMode: ExecutionModeNotSpecified,
		failure:       false,
		expected:      product.DotNetLinter.NativeAnalyzer(),
	},
	{
		name:          "--ide passed with EapSuffix",
		ide:           "QDNET-EAP",
		linter:        "",
		image:         "",
		executionMode: ExecutionModeNotSpecified,
		failure:       false,
		expected:      &product.NativeAnalyzer{Linter: product.DotNetLinter, Eap: true},
	},
	{
		name:          "--ide passed, --within-docker=true ignored",
		ide:           "QDNET",
		linter:        "",
		image:         "",
		executionMode: ExecutionModeDocker,
		failure:       false,
		expected:      product.DotNetLinter.NativeAnalyzer(),
	},
	{
		name:          "Image passed to --linter",
		ide:           "",
		linter:        "jetbrains/qodana-dotnet:2023.3-eap",
		image:         "",
		executionMode: ExecutionModeNotSpecified,
		failure:       false,
		expected: &product.DockerAnalyzer{
			Linter: product.DotNetLinter,
			Image:  "jetbrains/qodana-dotnet:2023.3-eap",
		},
	},
	{
		name:          "Internal image passed to --linter",
		ide:           "",
		linter:        "registry.jetbrains.team/p/sa/containers/qodana-php:2023.3-rc",
		image:         "",
		executionMode: ExecutionModeNotSpecified,
		failure:       false,
		expected: &product.DockerAnalyzer{
			Linter: product.PhpLinter,
			Image:  "registry.jetbrains.team/p/sa/containers/qodana-php:2023.3-rc",
		},
	},
	{
		name:          "--ide and linter passed to --linter",
		ide:           "QDNET",
		linter:        "jetbrains/qodana-php:2023.3-eap",
		image:         "",
		executionMode: ExecutionModeNotSpecified,
		failure:       true,
		expected:      nil,
	},
	{
		name:          "--ide and image passed to --linter",
		ide:           "QDNET",
		linter:        "",
		image:         "jetbrains/qodana-php:2023.3-eap",
		executionMode: ExecutionModeNotSpecified,
		failure:       true,
		expected:      nil,
	},
	{
		name:          "Unknown image passed to --linter",
		ide:           "",
		linter:        "jetbrains/qodana-unknown:2023.3-eap",
		image:         "",
		executionMode: ExecutionModeNotSpecified,
		failure:       true,
		expected:      nil,
	},
	{
		name:          "Known image passed --image",
		ide:           "",
		linter:        "",
		image:         "jetbrains/qodana-python:2023.3",
		executionMode: ExecutionModeNotSpecified,
		failure:       false,
		expected: &product.DockerAnalyzer{
			Linter: product.PythonLinter,
			Image:  "jetbrains/qodana-python:2023.3",
		},
	},
	{
		name:          "Known image passed --image, --within-docker=false ignored",
		ide:           "",
		linter:        "",
		image:         "jetbrains/qodana-python:2023.3",
		executionMode: ExecutionModeNative,
		failure:       false,
		expected: &product.DockerAnalyzer{
			Linter: product.PythonLinter,
			Image:  "jetbrains/qodana-python:2023.3",
		},
	},
	{
		name:          "Image passed to --linter, --within-docker=false ignored",
		ide:           "",
		linter:        "jetbrains/qodana-go:2025.2",
		image:         "",
		executionMode: ExecutionModeNative,
		failure:       false,
		expected:      &product.DockerAnalyzer{Linter: product.GoLinter, Image: "jetbrains/qodana-go:2025.2"},
	},
	{
		name:          "Known image passed to --image",
		ide:           "",
		linter:        "",
		image:         "myorg/qodana-jvm:2023.3",
		executionMode: ExecutionModeNotSpecified,
		failure:       false,
		expected:      &product.DockerAnalyzer{Linter: product.UnknownLinter, Image: "myorg/qodana-jvm:2023.3"},
	},
	{
		name:          "Known image passed to --image and --linter is ignored",
		ide:           "",
		linter:        "qodana-js",
		image:         "jetbrains/qodana-go:2025.2",
		executionMode: ExecutionModeNotSpecified,
		failure:       false,
		expected:      &product.DockerAnalyzer{Linter: product.GoLinter, Image: "jetbrains/qodana-go:2025.2"},
	},
	{
		name:          "Known linter passed --linter, --within-docker empty",
		ide:           "",
		linter:        "qodana-js",
		image:         "",
		executionMode: ExecutionModeNotSpecified,
		failure:       false,
		expected: &product.DockerAnalyzer{
			Linter: product.JsLinter,
			Image:  product.JsLinter.Image(),
		},
	},
	{
		name:          "Known linter passed --linter, --within-docker=false",
		ide:           "",
		linter:        "qodana-js",
		image:         "",
		executionMode: ExecutionModeNative,
		failure:       false,
		expected:      &product.NativeAnalyzer{Linter: product.JsLinter, Eap: !product.IsReleased},
	},
	{
		name:          "Known linter passed --linter, --within-docker=false, cpp linter",
		ide:           "",
		linter:        "qodana-cpp",
		image:         "",
		executionMode: ExecutionModeNative,
		failure:       false,
		expected: &product.NativeAnalyzer{
			Linter: product.CppLinter,
			Eap:    !product.IsReleased || product.CppLinter.EapOnly,
		},
	},
	{
		name:          "Known linter passed --linter, --within-docker=true",
		ide:           "",
		linter:        "qodana-js",
		image:         "",
		executionMode: ExecutionModeDocker,
		failure:       false,
		expected: &product.DockerAnalyzer{
			Linter: product.JsLinter,
			Image:  product.JsLinter.Image(),
		},
	},
	{
		name:          "Unknown linter passed to --linter",
		ide:           "",
		linter:        "super-linter",
		image:         "",
		executionMode: ExecutionModeDocker,
		failure:       true,
		expected:      nil,
	},
}
