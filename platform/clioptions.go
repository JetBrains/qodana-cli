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

package platform

import (
	"github.com/spf13/pflag"
)

// ThirdPartyOptions is used to customize the CLI options for a specific linter.
type ThirdPartyOptions interface {
	AddFlags(flags *pflag.FlagSet)
	GetMountInfo() *MountInfo
	MountTools(tempPath string, mountPath string, o *QodanaOptions) (map[string]string, error)
	GetInfo(o *QodanaOptions) *LinterInfo
	Setup(o *QodanaOptions) error
	RunAnalysis(o *QodanaOptions) error
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

type LinterSpecificInitializer func() ThirdPartyOptions

func DefineOptions(initializer LinterSpecificInitializer) *QodanaOptions {
	options := &QodanaOptions{}
	if initializer != nil {
		options.LinterSpecific = initializer()
	}
	return options
}

func (o *QodanaOptions) GetLinterSpecificOptions() *ThirdPartyOptions {
	if o.LinterSpecific != nil {
		if linterSpecific, ok := o.LinterSpecific.(ThirdPartyOptions); ok {
			return &linterSpecific
		}
	}
	return nil
}

func (o *QodanaOptions) GetLinterInfo() *LinterInfo {
	linterOpts := o.GetLinterSpecificOptions()
	if linterOpts != nil {
		return (*linterOpts).GetInfo(o)
	}
	return nil
}
