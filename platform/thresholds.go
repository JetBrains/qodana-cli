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
	"github.com/JetBrains/qodana-cli/v2024/platform/thirdpartyscan"
	"strconv"
)

const severityAny = "any"
const severityCritical = "critical"
const severityHigh = "high"
const severityModerate = "moderate"
const severityLow = "low"
const severityInfo = "info"

func getFailureThresholds(c thirdpartyscan.Context) map[string]string {
	yaml := c.QodanaYamlConfig()
	ret := make(map[string]string)
	if yaml.FailThreshold != nil {
		ret[severityAny] = strconv.Itoa(*yaml.FailThreshold)
	}
	if yaml.FailureConditions.SeverityThresholds != nil {
		thresholds := *yaml.FailureConditions.SeverityThresholds
		if thresholds.Any != nil {
			ret[severityAny] = strconv.Itoa(*thresholds.Any)
		}
		if thresholds.Critical != nil {
			ret[severityCritical] = strconv.Itoa(*thresholds.Critical)
		}
		if thresholds.High != nil {
			ret[severityHigh] = strconv.Itoa(*thresholds.High)
		}
		if thresholds.Moderate != nil {
			ret[severityModerate] = strconv.Itoa(*thresholds.Moderate)
		}
		if thresholds.Low != nil {
			ret[severityLow] = strconv.Itoa(*thresholds.Low)
		}
		if thresholds.Info != nil {
			ret[severityInfo] = strconv.Itoa(*thresholds.Info)
		}
	}
	if c.FailThreshold() != "" { // console option overrides the behavior
		ret = make(map[string]string)
		ret[severityAny] = c.FailThreshold()
	}
	return ret
}

func thresholdsToArgs(thresholds map[string]string) []string {
	args := make([]string, 0)
	for severity, value := range thresholds {
		args = append(args, fmt.Sprintf("--threshold-%s=%s", severity, value))
	}
	return args
}
