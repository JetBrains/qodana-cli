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
	"os"
	"testing"
)

func TestMount(t *testing.T) {
	linter := mockThirdPartyLinter{}
	tempCacheDir, _ := os.MkdirTemp("", "qodana-platform")
	defer func() {
		_ = os.RemoveAll(tempCacheDir)
	}()

	tempMountPath, mountInfo := extractUtils(linter, tempCacheDir, false)
	defer cleanupUtils(tempMountPath)

	if mountInfo.Converter == "" {
		t.Error("extractUtils() failed")
	}

	list := []string{mountInfo.Converter, mountInfo.Fuser, mountInfo.BaselineCli}
	// TODO: should be per-linter test as well
	for _, v := range mountInfo.CustomTools {
		list = append(list, v)
	}

	for _, p := range list {
		_, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				t.Error("Unpacking failed")
			}
		}
	}
}

type mockThirdPartyLinter struct {
}

func (mockThirdPartyLinter) MountTools(_ string, _ string, _ bool) (map[string]string, error) {
	return make(map[string]string), nil
}
func (mockThirdPartyLinter) ComputeNewLinterInfo(
	info thirdpartyscan.LinterInfo,
	_ bool,
) (thirdpartyscan.LinterInfo, error) {
	return info, nil
}

func (mockThirdPartyLinter) RunAnalysis(_ thirdpartyscan.Context) error {
	return nil
}
