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
