/*
 * Copyright 2021-2023 JetBrains s.r.o.
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

package core

import (
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

var (
	defaultEndpoint = "qodana.cloud"
	defaultService  = "qodana-cli"
)

// getCloudTeamsPageUrl returns the team page URL on Qodana Cloud
func getCloudTeamsPageUrl(path string) string {
	name := filepath.Base(path)
	origin := gitRemoteUrl(path)

	return strings.Join([]string{"https://", defaultEndpoint, "/?origin=", origin, "&name=", name}, "")
}

// saveCloudToken saves token to the system keyring
func saveCloudToken(id string, token string) error {
	err := keyring.Set(defaultService, id, token)
	if err != nil {
		return err
	}
	return nil
}

// getCloudToken returns token from the system keyring
func getCloudToken(id string) (string, error) {
	secret, err := keyring.Get(defaultService, id)
	if err != nil {
		return "", err
	}
	return secret, nil
}
