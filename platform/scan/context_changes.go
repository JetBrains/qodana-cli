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

package scan

import (
	"fmt"
	"strings"
)

func (c Context) WithAddedProperties(propertiesToAdd ...string) Context {
	props := c.Property()
	props = append(props, propertiesToAdd...)
	c._property = props
	return c
}

func (c Context) WithEnvOverride(key string, value string) Context {
	return c.withEnv(key, value, true)
}

func (c Context) WithEnvNoOverride(key string, value string) Context {
	return c.withEnv(key, value, false)
}

func (c Context) withEnv(key string, value string, override bool) Context {
	currentEnvs := c.Env()
	envs := make([]string, len(currentEnvs))

	for _, e := range currentEnvs {
		isEnvAlreadySet := strings.HasPrefix(e, key) && value != ""
		if isEnvAlreadySet && !override {
			return c
		}

		if !isEnvAlreadySet {
			envs = append(envs, e)
		}
	}
	if value != "" {
		envs = append(envs, fmt.Sprintf("%s=%s", key, value))
	}

	c._env = envs
	return c
}
