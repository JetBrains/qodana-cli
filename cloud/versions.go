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
	log "github.com/sirupsen/logrus"
	"net/http"
	"strconv"
	"strings"
)

const (
	RequiredMajorVersion        = 1
	MinimumRequiredMinorVersion = 0
	VersionsURI                 = "/api/versions"
)

type APIVersion struct {
	Major int
	Minor int
}

type ApiVersionMismatchError struct {
	ApiKind           string
	SupportedVersions []string
}

func (e *ApiVersionMismatchError) Error() string {
	return fmt.Sprintf(
		"failed to find supported API. Available %s API: %v. Required major version: %d. Minimum required minor version: %d",
		e.ApiKind,
		e.SupportedVersions,
		RequiredMajorVersion,
		MinimumRequiredMinorVersion,
	)
}

func ToCloudVersion(version string) (APIVersion, error) {
	versionParts := strings.Split(version, ".")
	if len(versionParts) != 2 {
		return APIVersion{}, errors.New("invalid version format")
	}
	major, err := strconv.Atoi(versionParts[0])
	if err != nil {
		return APIVersion{}, errors.New("invalid major version")
	}
	minor, err := strconv.Atoi(versionParts[1])
	if err != nil {
		return APIVersion{}, errors.New("invalid minor version")
	}
	return APIVersion{
		Major: major,
		Minor: minor,
	}, nil
}

type ApiEndpointDescription struct {
	Versions []ApiVersionDescription `json:"versions"`
}

type ApiVersionDescription struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

type ApiDescriptions struct {
	API     ApiEndpointDescription `json:"api"`
	Linters ApiEndpointDescription `json:"linters"`
}

func selectSupportedVersion(apiDescriptions []ApiVersionDescription) string {
	for _, version := range apiDescriptions {
		cloudVersion, err := ToCloudVersion(version.Version)
		if err != nil {
			log.Fatalf("Failed to parse cloud version: %v", err)
		}
		if cloudVersion.Major == RequiredMajorVersion && cloudVersion.Minor >= MinimumRequiredMinorVersion {
			return version.URL

		}
	}
	return ""
}

func (endpoint *QdRootEndpoint) requestApiEndpoints() (*QdApiEndpoints, error) {
	httpClient := &http.Client{
		Timeout: getRequestTimeout(),
	}

	return endpoint.requestApiEndpointsCustomClient(httpClient)
}

func (endpoint *QdRootEndpoint) requestApiEndpointsCustomClient(httpClient *http.Client) (*QdApiEndpoints, error) {
	client := QdClient{
		httpClient: httpClient,
		apiUrl:     endpoint.Url,
	}

	request := NewCloudRequest(VersionsURI)

	response, err := client.doRequest(&request)
	if err != nil {
		return nil, fmt.Errorf("request of available API versions failed: %w", err)
	}

	var apiDescriptions ApiDescriptions
	err = json.Unmarshal(response, &apiDescriptions)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal API descriptions: %w", err)
	}

	apiEndpoints := QdApiEndpoints{RootEndpoint: endpoint}
	apiEndpoints.CloudApiUrl = selectSupportedVersion(apiDescriptions.API.Versions)
	if apiEndpoints.CloudApiUrl == "" {
		return nil, &ApiVersionMismatchError{"cloud", extractVersions(apiDescriptions.API.Versions)}
	}
	apiEndpoints.LintersApiUrl = selectSupportedVersion(apiDescriptions.Linters.Versions)
	if apiEndpoints.LintersApiUrl == "" {
		return nil, &ApiVersionMismatchError{"linters", extractVersions(apiDescriptions.Linters.Versions)}
	}

	return &apiEndpoints, err
}

func extractVersions(descriptions []ApiVersionDescription) []string {
	var versions []string

	for _, apiVersion := range descriptions {
		versions = append(versions, apiVersion.Version)
	}

	return versions
}
