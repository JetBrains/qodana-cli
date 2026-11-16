/*
 * Copyright 2026 JetBrains s.r.o.
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

package corescan

import (
	"strings"

	log "github.com/sirupsen/logrus"
)

// AirGappedProperty is the JVM property that turns on air-gapped mode in the linter.
// The CLI passes the mode as this property, as it passes the settings of the scoped stages.
// GetScanProperties writes it for a native linter, and GetIdeArgs passes it to the CLI in a container.
const AirGappedProperty = "-Dqodana.air.gapped"

// isAirGappedByProperty tells if a CLI property or a property of qodana.yaml sets AirGappedProperty to true.
// The CLI in a container gets the mode from the CLI on the host only in this way.
func isAirGappedByProperty(cliProperties []string, yamlProperties map[string]string) bool {
	for _, p := range cliProperties {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 && normalizePropertyKey(kv[0]) == AirGappedProperty && strings.EqualFold(kv[1], "true") {
			return true
		}
	}
	return strings.EqualFold(yamlProperties[AirGappedProperty], "true")
}

func normalizePropertyKey(key string) string {
	if strings.HasPrefix(key, "-") {
		return key
	}
	return "-D" + key
}

func normalizeYamlProperties(properties map[string]string) map[string]string {
	normalized := make(map[string]string, len(properties))
	for key, value := range properties {
		normalizedKey := normalizePropertyKey(key)
		if currentValue, exists := normalized[normalizedKey]; !exists {
			normalized[normalizedKey] = value
		} else {
			log.Warnf("The %q specified in YAML properties two times, using value: %q", normalizedKey, currentValue)
		}
	}

	return normalized
}
