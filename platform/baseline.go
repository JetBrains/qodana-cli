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
	"fmt"
)

// computeBaselinePrintResults runs SARIF analysis (compares with baseline and prints the result)=
func computeBaselinePrintResults(options *QodanaOptions, mountInfo *MountInfo) (int, error) {
	args := []string{QuoteForWindows(mountInfo.JavaPath), "-jar", QuoteForWindows(mountInfo.BaselineCli), "-r", QuoteForWindows(options.GetSarifPath())}
	if options.FailThreshold != "" {
		args = append(args, "-f", options.FailThreshold)
	}
	if options.Baseline != "" {
		args = append(args, "-b", QuoteForWindows(options.Baseline))
	}
	ret, err := LaunchAndLog(options, "baseline", args...)
	if err != nil {
		return -1, fmt.Errorf("error while running baseline-cli: %w", err)
	}
	if ret > 0 {
		if ret == 1 {
			return -1, fmt.Errorf("error in supplied arguments for baseline-cli")
		}
		return ret, nil
	}
	return 0, err
}
