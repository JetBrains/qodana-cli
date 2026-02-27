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

package cloud

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// GetCloudTeamsPageUrl returns the team page URL on Qodana Cloud
func (endpoint *QdRootEndpoint) GetCloudTeamsPageUrl(origin string, path string) string {
	name := filepath.Base(path)
	return fmt.Sprintf("%s/?origin=%s&name=%s", endpoint.Url, origin, name)
}

func (client *QdClient) RequestProjectName() (string, error) {
	request := NewCloudRequest("/projects")
	result, err := client.doRequest(&request)
	if err != nil {
		return "", err
	}
	projectName, err := parseProjectName(result)
	if err != nil {
		return "", err
	}
	return projectName, nil
}

func parseProjectName(data []byte) (string, error) {
	var answer map[string]any
	if err := json.Unmarshal(data, &answer); err != nil {
		return "", fmt.Errorf("response '%s': %w", string(data), err)
	}
	return fmt.Sprintf("%v", answer["name"]), nil
}
