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
	"github.com/JetBrains/qodana-cli/v2024/platform/thirdpartyscan"
)

type ThirdPartyLinter interface {
	MountTools(tempPath string, mountPath string, isCommunity bool) (map[string]string, error)
	ComputeNewLinterInfo(info thirdpartyscan.LinterInfo, isCommunity bool) (thirdpartyscan.LinterInfo, error)
	RunAnalysis(c thirdpartyscan.Context) error
}

type LinterSpecificInitializer func() ThirdPartyLinter

func DefineOptions(initializer LinterSpecificInitializer) *QodanaOptions {
	options := &QodanaOptions{}
	if initializer != nil {
		options.LinterSpecific = initializer()
	}
	return options
}

func (o *QodanaOptions) GetLinterSpecificOptions() *ThirdPartyLinter {
	if o.LinterSpecific != nil {
		if linterSpecific, ok := o.LinterSpecific.(ThirdPartyLinter); ok {
			return &linterSpecific
		}
	}
	return nil
}
