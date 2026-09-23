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
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"
)

// Qodana Public API: https://github.com/jetbrains-qodana/public-api/blob/main/openapi.yaml
// It is served under the Cloud API URL and authenticated with an organization API token.
const publicProjectsPath = "/public/organizations/projects"

var OrgTokenDeclinedError = errors.New("organization token was declined by Qodana Cloud server")

const OrgTokenExchangeFailedMessage = `Failed to obtain a project token using QODANA_ORG_TOKEN for project '%s'.
Please ensure that QODANA_ORG_TOKEN is a valid organization API token and that QODANA_PROJECT_SLUG or 'projectSlug:' in qodana.yaml is correct.`

// QdPublicApiClient is a client of the Qodana Public API, authenticated with an organization API token.
// It is a separate type from QdClient so that the organization token is never used for project-token endpoints.
type QdPublicApiClient struct {
	client *QdClient
}

type projectTokenRequest struct {
	ProjectQualifiedSlug string `json:"projectQualifiedSlug"`
	TokenTtlSeconds      int64  `json:"tokenTtlSeconds,omitempty"`
}

type projectTokenResponse struct {
	ProjectToken string `json:"projectToken"`
}

func (endpoints *QdApiEndpoints) NewPublicApiClient(orgToken string) *QdPublicApiClient {
	return &QdPublicApiClient{client: endpoints.NewCloudApiClient(orgToken)}
}

// RequestProjectToken returns a project token valid for ttl for the project given as team-slug:project-slug.
// Note: Qodana Cloud creates the team and project if they don't exist.
func (c *QdPublicApiClient) RequestProjectToken(projectQualifiedSlug string, ttl time.Duration) (string, error) {
	body, err := json.Marshal(
		projectTokenRequest{
			ProjectQualifiedSlug: projectQualifiedSlug,
			TokenTtlSeconds:      int64(ttl.Seconds()),
		},
	)
	if err != nil {
		return "", err
	}
	request := NewCloudRequest(publicProjectsPath)
	request.Method = http.MethodPost
	request.Body = body
	request.AcceptedStatuses = append(request.AcceptedStatuses, http.StatusBadRequest, http.StatusForbidden)

	result, err := c.client.doRequest(&request)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && slices.Contains(request.AcceptedStatuses, apiErr.StatusCode) {
			return "", fmt.Errorf("%w: %v", OrgTokenDeclinedError, err)
		}
		return "", err
	}
	return parseProjectTokenResponse(result)
}

func parseProjectTokenResponse(data []byte) (string, error) {
	var answer projectTokenResponse
	// don't include the response in errors: it contains a token
	if err := json.Unmarshal(data, &answer); err != nil {
		return "", errors.New("failed to parse project token response")
	}
	if answer.ProjectToken == "" {
		return "", errors.New("project token response contains no token")
	}
	return answer.ProjectToken, nil
}
