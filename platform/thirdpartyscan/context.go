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

package thirdpartyscan

import (
	"github.com/JetBrains/qodana-cli/v2024/platform/product"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdyaml"
	"regexp"
)

const Clang = "clang"
const Clt = "clt"

type ThirdPartyStartupCloudData struct {
	LicensePlan   string
	ProjectIdHash string
	QodanaToken   string
}

// MountInfo is a struct that contains all the helper tools to run a Qodana linter.
type MountInfo struct {
	Converter   string
	Fuser       string
	BaselineCli string
	CustomTools map[string]string
	JavaPath    string
}

// LinterInfo is a struct that contains all the information about the linter.
type LinterInfo struct {
	ProductCode   string
	LinterName    string
	LinterVersion string
	IsEap         bool
}

// Context
//
// !!!KEEP IT IMMUTABLE!!!
// !!!KEEP IT IMMUTABLE!!!
// !!!KEEP IT IMMUTABLE!!!
//
// If one has the instance of Context, then it means that it was initialized, and it is in valid state
// all mutations should be defined in context_changes.go with names clearly demonstrating the usecase and business logic.
// example: scoped script launches two stages of default analysis
//
// # In the future, you can consider making it an interface
//
// Yes, there is a lot of boilerplate, and it's ok, it's much better than having one object written and initialized across your program
// if I have an object it means it must be in valid state, otherwise it's impossible to reason about the code
//
// Q: - Why do we prohibit mutations in Context if we pass it by value already?
// A: - Because we want to prohibit any unexpected changes to Context at all, imagine at some place the projectDir will
//
//	suddenly change, and pass it further? It's not clear why it was changed? Was it actually initialized at this place?
//	but it must be initialized already, so we limit all changes, and keep them in context_changes.go
//
// !!!KEEP IT IMMUTABLE!!!
// !!!KEEP IT IMMUTABLE!!!
// !!!KEEP IT IMMUTABLE!!!
type Context struct {
	linterInfo            LinterInfo
	mountInfo             MountInfo
	cloudData             ThirdPartyStartupCloudData
	projectDir            string
	resultsDir            string
	logDir                string
	cacheDir              string
	clangCompileCommands  string
	clangArgs             string
	property              []string
	cdnetSolution         string
	cdnetProject          string
	cdnetConfiguration    string
	cdnetPlatform         string
	noStatistics          bool
	cdnetNoBuild          bool
	analysisId            string
	baseline              string
	baselineIncludeAbsent bool
	failThreshold         string
	qodanaYaml            qdyaml.QodanaYaml
}

type ContextBuilder struct {
	LinterInfo            LinterInfo
	MountInfo             MountInfo
	CloudData             ThirdPartyStartupCloudData
	ProjectDir            string
	ResultsDir            string
	LogDir                string
	CacheDir              string
	ClangCompileCommands  string
	ClangArgs             string
	Property              []string
	CdnetSolution         string
	CdnetProject          string
	CdnetConfiguration    string
	CdnetPlatform         string
	NoStatistics          bool
	CdnetNoBuild          bool
	AnalysisId            string
	Baseline              string
	BaselineIncludeAbsent bool
	FailThreshold         string
	QodanaYaml            qdyaml.QodanaYaml
}

func (b ContextBuilder) Build() Context {
	return Context{
		linterInfo:            b.LinterInfo,
		mountInfo:             b.MountInfo,
		cloudData:             b.CloudData,
		projectDir:            b.ProjectDir,
		resultsDir:            b.ResultsDir,
		logDir:                b.LogDir,
		cacheDir:              b.CacheDir,
		clangCompileCommands:  b.ClangCompileCommands,
		clangArgs:             b.ClangArgs,
		property:              b.Property,
		cdnetSolution:         b.CdnetSolution,
		cdnetProject:          b.CdnetProject,
		cdnetConfiguration:    b.CdnetConfiguration,
		cdnetPlatform:         b.CdnetPlatform,
		noStatistics:          b.NoStatistics,
		cdnetNoBuild:          b.CdnetNoBuild,
		analysisId:            b.AnalysisId,
		baseline:              b.Baseline,
		baselineIncludeAbsent: b.BaselineIncludeAbsent,
		failThreshold:         b.FailThreshold,
	}
}

func (c Context) LinterInfo() LinterInfo                { return c.linterInfo }
func (c Context) MountInfo() MountInfo                  { return c.mountInfo }
func (c Context) CloudData() ThirdPartyStartupCloudData { return c.cloudData }
func (c Context) ProjectDir() string                    { return c.projectDir }
func (c Context) ResultsDir() string                    { return c.resultsDir }
func (c Context) LogDir() string                        { return c.logDir }
func (c Context) CacheDir() string                      { return c.cacheDir }
func (c Context) ClangCompileCommands() string          { return c.clangCompileCommands }
func (c Context) ClangArgs() string                     { return c.clangArgs }
func (c Context) CdnetSolution() string                 { return c.cdnetSolution }
func (c Context) CdnetProject() string                  { return c.cdnetProject }
func (c Context) CdnetConfiguration() string            { return c.cdnetConfiguration }
func (c Context) CdnetPlatform() string                 { return c.cdnetPlatform }
func (c Context) NoStatistics() bool                    { return c.noStatistics }
func (c Context) CdnetNoBuild() bool                    { return c.cdnetNoBuild }
func (c Context) AnalysisId() string                    { return c.analysisId }
func (c Context) Baseline() string                      { return c.baseline }
func (c Context) BaselineIncludeAbsent() bool           { return c.baselineIncludeAbsent }
func (c Context) FailThreshold() string                 { return c.failThreshold }
func (c Context) QodanaYaml() qdyaml.QodanaYaml         { return c.qodanaYaml }

func (c Context) Property() []string {
	props := make([]string, len(c.property))
	copy(props, c.property)
	return props
}

func (c Context) IsCommunity() bool {
	return c.CloudData().LicensePlan == "COMMUNITY"
}

func (c Context) ClangPath() string {
	return c.MountInfo().CustomTools[Clang]
}

func (i LinterInfo) GetMajorVersion() string {
	re := regexp.MustCompile(`\b\d+\.\d+`)
	matches := re.FindStringSubmatch(i.LinterVersion)
	if len(matches) == 0 {
		return product.ReleaseVersion
	}
	return matches[0]
}
