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
	"github.com/JetBrains/qodana-cli/v2025/platform/strutil"
	"strings"
)

type Linter struct {
	Name            string
	PresentableName string
	ProductCode     string
	DockerImage     string
	SupportNative   bool
	IsPaid          bool
	SupportFixes    bool
	EapOnly         bool
}

const (
	ReleaseVersion = "2025.1"
	ShortVersion   = "251"
	IsReleased     = true

	EapSuffix  = "-EAP"
	ReleaseVer = "release"
	EapVer     = "eap"

	QDJVMC = "QDJVMC"
	QDJVM  = "QDJVM"
	QDAND  = "QDAND"
	QDPHP  = "QDPHP"
	QDPY   = "QDPY"
	QDPYC  = "QDPYC"
	QDJS   = "QDJS"
	QDGO   = "QDGO"
	QDNET  = "QDNET"
	QDNETC = "QDNETC"
	QDANDC = "QDANDC"
	QDRST  = "QDRST"
	QDRUBY = "QDRUBY"
	QDCLC  = "QDCLC"
	QDCPP  = "QDCPP"
)

var (
	UnknownLinter = Linter{}

	JvmLinter = Linter{
		PresentableName: "Qodana Ultimate for JVM",
		Name:            "qodana-jvm",
		ProductCode:     QDJVM,
		DockerImage:     "jetbrains/qodana-jvm",
		SupportNative:   true,
		IsPaid:          true,
		SupportFixes:    true,
		EapOnly:         false,
	}

	JvmCommunityLinter = Linter{
		PresentableName: "Qodana Community for JVM",
		Name:            "qodana-jvm-community",
		ProductCode:     QDJVMC,
		DockerImage:     "jetbrains/qodana-jvm-community",
		SupportNative:   true,
		IsPaid:          false,
		SupportFixes:    false,
		EapOnly:         false,
	}

	AndroidLinter = Linter{
		PresentableName: "Qodana for Android",
		Name:            "qodana-android",
		ProductCode:     QDAND,
		DockerImage:     "jetbrains/qodana-android",
		SupportNative:   false,
		IsPaid:          true,
		SupportFixes:    true,
		EapOnly:         false,
	}

	AndroidCommunityLinter = Linter{
		PresentableName: "Qodana Community for Android",
		Name:            "qodana-jvm-android",
		ProductCode:     QDANDC,
		DockerImage:     "jetbrains/qodana-jvm-android",
		SupportNative:   false,
		IsPaid:          false,
		SupportFixes:    false,
		EapOnly:         false,
	}

	PhpLinter = Linter{
		PresentableName: "Qodana for PHP",
		Name:            "qodana-php",
		ProductCode:     QDPHP,
		DockerImage:     "jetbrains/qodana-php",
		SupportNative:   true,
		IsPaid:          true,
		SupportFixes:    true,
		EapOnly:         false,
	}

	PythonLinter = Linter{
		PresentableName: "Qodana for Python",
		Name:            "qodana-python",
		ProductCode:     QDPY,
		DockerImage:     "jetbrains/qodana-python",
		SupportNative:   true,
		IsPaid:          true,
		SupportFixes:    true,
		EapOnly:         false,
	}

	PythonCommunityLinter = Linter{
		PresentableName: "Qodana Community for Python",
		Name:            "qodana-python-community",
		ProductCode:     QDPYC,
		DockerImage:     "jetbrains/qodana-python-community",
		SupportNative:   true,
		IsPaid:          false,
		SupportFixes:    false,
		EapOnly:         false,
	}

	JsLinter = Linter{
		PresentableName: "Qodana for JS",
		Name:            "qodana-js",
		ProductCode:     QDJS,
		DockerImage:     "jetbrains/qodana-js",
		SupportNative:   true,
		IsPaid:          true,
		SupportFixes:    true,
		EapOnly:         false,
	}

	DotNetLinter = Linter{
		PresentableName: "Qodana for .NET",
		Name:            "qodana-dotnet",
		ProductCode:     QDNET,
		DockerImage:     "jetbrains/qodana-dotnet",
		SupportNative:   true,
		IsPaid:          true,
		SupportFixes:    true,
		EapOnly:         false,
	}

	RubyLinter = Linter{
		PresentableName: "Qodana for Ruby",
		Name:            "qodana-ruby",
		ProductCode:     QDRUBY,
		DockerImage:     "jetbrains/qodana-ruby",
		SupportNative:   false,
		IsPaid:          true,
		SupportFixes:    true,
		EapOnly:         true,
	}

	CppLinter = Linter{
		PresentableName: "Qodana for C/C++",
		Name:            "qodana-cpp",
		ProductCode:     QDCPP,
		DockerImage:     "jetbrains/qodana-cpp",
		SupportNative:   false,
		IsPaid:          true,
		SupportFixes:    false,
		EapOnly:         true,
	}

	GoLinter = Linter{
		PresentableName: "Qodana for Go",
		Name:            "qodana-go",
		ProductCode:     QDGO,
		DockerImage:     "jetbrains/qodana-go",
		SupportNative:   true,
		IsPaid:          true,
		SupportFixes:    true,
		EapOnly:         false,
	}

	DotNetCommunityLinter = Linter{
		PresentableName: "Qodana Community for .NET",
		Name:            "qodana-dotnet-community",
		ProductCode:     QDNETC,
		DockerImage:     "jetbrains/qodana-dotnet-community",
		SupportNative:   false,
		IsPaid:          false,
		SupportFixes:    false,
		EapOnly:         true,
	}

	ClangLinter = Linter{
		PresentableName: "Qodana Community for C/C++",
		Name:            "qodana-clang",
		ProductCode:     QDCLC,
		DockerImage:     "jetbrains/qodana-clang",
		SupportNative:   false,
		IsPaid:          false,
		SupportFixes:    false,
		EapOnly:         true,
	}

	VersionsMap = map[string]string{
		ReleaseVer: "2025.1",
		EapVer:     "2024.3",
	}

	Products = map[string]string{
		QDJVM:  "IIU",
		QDJVMC: "IIC",
		// QDAND: // don't use it right now
		// QDANDC: // don't use it right now
		QDPHP:  "PS",
		QDJS:   "WS",
		QDNET:  "RD",
		QDPY:   "PCP",
		QDPYC:  "PCC",
		QDGO:   "GO",
		QDRUBY: "RM",
		QDRST:  "RR",
		QDCPP:  "CL",
	}

	// AllNativeCodes is a list of all supported Qodana linters product codes
	AllNativeCodes = []string{QDNET, QDJVM, QDJVMC, QDGO, QDPY, QDPYC, QDJS, QDPHP}

	// AllLinters Order is important for detection
	AllLinters = []Linter{
		JvmCommunityLinter,
		JvmLinter,
		AndroidCommunityLinter,
		AndroidLinter,
		PhpLinter,
		PythonCommunityLinter,
		PythonLinter,
		JsLinter,
		DotNetCommunityLinter,
		DotNetLinter,
		RubyLinter,
		CppLinter,
		GoLinter,
		ClangLinter,
	}
)

func (linter *Linter) NativeAnalyzer() Analyzer {
	return &NativeAnalyzer{
		Linter: *linter,
		Ide:    linter.ProductCode,
	}
}

func (linter *Linter) DockerAnalyzer() Analyzer {
	return &DockerAnalyzer{
		Linter: *linter,
		Image:  linter.Image(),
	}
}

func (linter *Linter) Image() string {
	//goland:noinspection GoBoolExpressions
	if !IsReleased || linter.EapOnly {
		return linter.DockerImage + ":" + ReleaseVersion + "-eap"
	}
	return linter.DockerImage + ":" + ReleaseVersion
}

// LangsToLinters is a map of languages to linters.
var LangsToLinters = map[string][]Linter{
	"Java": {
		JvmLinter,
		JvmCommunityLinter,
		AndroidLinter,
		AndroidCommunityLinter,
	},
	"Kotlin": {
		JvmLinter,
		JvmCommunityLinter,
		AndroidLinter,
		AndroidCommunityLinter,
	},
	"PHP":               {PhpLinter},
	"Python":            {PythonLinter, PythonCommunityLinter},
	"JavaScript":        {JsLinter},
	"TypeScript":        {JsLinter},
	"Go":                {GoLinter},
	"C#":                {DotNetLinter, DotNetCommunityLinter},
	"F#":                {DotNetLinter},
	"Visual Basic .NET": {DotNetLinter, DotNetCommunityLinter},
	"C":                 {CppLinter, ClangLinter, DotNetLinter},
	"C++":               {CppLinter, ClangLinter, DotNetLinter},
	"Ruby":              {RubyLinter},
}

var AllSupportedPaidLinters = allLintersFiltered(AllLinters, func(linter *Linter) bool { return linter.IsPaid })
var AllSupportedFreeLinters = allLintersFiltered(AllLinters, func(linter *Linter) bool { return !linter.IsPaid })
var AllNativeLinters = allLintersFiltered(AllLinters, func(linter *Linter) bool { return linter.SupportNative })

var AllFixesSupportedProducts = []string{QDJVM, QDNET, QDPY, QDJS, QDPHP, QDGO, QDAND, QDRUBY}

func allImages(linters []Linter) []string {
	var images []string
	for _, linter := range linters {
		images = append(images, linter.Image())
	}
	return images
}

func allLintersFiltered(linters []Linter, filter func(linter *Linter) bool) []Linter {
	var filtered []Linter
	for i := range linters {
		if filter(&linters[i]) {
			filtered = append(filtered, linters[i])
		}
	}
	return filtered
}

// AllImages is a list of all supported linters.
var AllImages = allImages(AllLinters)

// FindByImageName returns the Linter for a given Docker image name
func FindByImageName(image string) Linter {
	if image == "" {
		return UnknownLinter
	}
	for _, linter := range AllLinters {
		if strings.Contains(image, linter.DockerImage) {
			return linter
		}
	}
	return UnknownLinter
}

// FindByProductCode returns the Linter for a given product code
func FindByProductCode(product string) Linter {
	if product == "" {
		return UnknownLinter
	}
	for _, linter := range AllLinters {
		if product == linter.ProductCode {
			return linter
		}
	}
	return UnknownLinter
}

// TODO check
// GuessLinter returns the Linter based on IDE product code or Docker image name
func GuessLinter(ide string, linterParam string) Linter {
	if ide != "" {
		productCode := strings.TrimSuffix(ide, EapSuffix)
		for _, linter := range AllLinters {
			if productCode == linter.ProductCode {
				return linter
			}
		}
		return UnknownLinter
	} else if linterParam != "" {
		linterParam := strings.TrimPrefix(linterParam, "https://")
		if strings.HasPrefix(linterParam, "registry.jetbrains.team/p/sa/containers/") {
			linterParam = strings.TrimPrefix(linterParam, "registry.jetbrains.team/p/sa/containers/")
			linterParam = "jetbrains/" + linterParam
		}
		for _, linter := range AllLinters {
			if strings.HasPrefix(linterParam, linter.DockerImage) {
				return linter
			}
		}
	}
	return UnknownLinter
}

func IsNativeAnalyzer(analyzer string) bool {
	return strutil.Contains(AllNativeCodes, analyzer)
}
