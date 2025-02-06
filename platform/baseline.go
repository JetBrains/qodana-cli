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
	thirdpartyscan "github.com/JetBrains/qodana-cli/v2024/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/v2024/platform/utils"
)

// computeBaselinePrintResults runs SARIF analysis (compares with baseline and prints the result)=
func computeBaselinePrintResults(c thirdpartyscan.Context, thresholds map[string]string) (int, error) {
	sarifPath := GetSarifPath(c.ResultsDir())
	args := []string{
		utils.QuoteForWindows(c.MountInfo().JavaPath),
		"-jar",
		utils.QuoteForWindows(c.MountInfo().BaselineCli),
		"-r",
		utils.QuoteForWindows(sarifPath),
	}
	severities := thresholdsToArgs(thresholds)
	for _, sev := range severities {
		args = append(args, sev)
	}
	if c.Baseline() != "" {
		args = append(args, "-b", utils.QuoteForWindows(c.Baseline()))
	}
	if c.BaselineIncludeAbsent() {
		args = append(args, "-i")
	}
	_, _, ret, err := utils.LaunchAndLog(c.LogDir(), "baseline", args...)
	if err != nil {
		return -1, fmt.Errorf("error while running baseline-cli: %w", err)
	}
	if ret > 0 {
		if ret == 1 {
			return -1, fmt.Errorf("error in supplied arguments for baseline-cli")
		}
		return ret, nil
	}
	return ret, nil
}
