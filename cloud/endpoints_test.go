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
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdenv"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
)

func TestEndpoint(t *testing.T) {
	for _, testData := range []struct {
		name  string
		input string
		host  string
		error bool
	}{
		{
			name:  "default",
			input: "",
			host:  DefaultEndpoint,
		},
		{
			name:  "with schema",
			input: "https://qodana.cloud",
			host:  DefaultEndpoint,
		},
		{
			name:  "with path",
			input: "https://qodana.cloud/api/v1",
			host:  DefaultEndpoint,
		},
		{
			name:  "custom domain",
			input: "https://qodana.company.com/api/v1",
			host:  "qodana.company.com",
		},
		{
			name:  "not domain",
			input: ":hsa/sdf",
			host:  "",
			error: true,
		},
	} {
		t.Run(
			testData.name, func(t *testing.T) {
				t.Cleanup(
					func() {
						log.StandardLogger().ExitFunc = os.Exit
						endpoint = nil
					},
				)

				if testData.input != "" {
					t.Setenv(qdenv.QodanaEndpointEnv, testData.input)
				}
				var fatal = false
				log.StandardLogger().ExitFunc = func(int) { fatal = true }

				cloudEndpoint := GetCloudRootEndpoint()
				if testData.error && !fatal {
					t.Errorf("Should be fatal")
				}
				if !testData.error && cloudEndpoint.Host != testData.host {
					assert.Equal(t, testData.host, cloudEndpoint.Host)
				}
			},
		)
	}
}

func TestObtainEndpointAPI(t *testing.T) {
	for _, testData := range []struct {
		name           string
		cooldown       int
		failedAttempts int
		actualAttempts int
		response       string
		cloudApiUrl    string
		lintersApiUrl  string
		success        bool
	}{
		{
			name:           "perfect server",
			cooldown:       0,
			failedAttempts: 0,
			actualAttempts: 1,
			response:       `{"api":{"versions":[{"version":"1.1","url":"https://api.qodana.cloud/v1"}]},"linters":{"versions":[{"version":"1.0","url":"https://linters.sssa.aws.intellij.net/v1"}]}}`,
			cloudApiUrl:    `https://api.qodana.cloud/v1`,
			lintersApiUrl:  `https://linters.sssa.aws.intellij.net/v1`,
			success:        true,
		},
		{
			name:           "retry on wrong response",
			cooldown:       1,
			failedAttempts: 2,
			actualAttempts: 3,
			response:       `{"api":{"versions":[{"version":"1.1","url":"https://api.qodana.cloud/v1"}]},"linters":{"versions":[{"version":"1.0","url":"https://linters.sssa.aws.intellij.net/v1"}]}}`,
			cloudApiUrl:    `https://api.qodana.cloud/v1`,
			lintersApiUrl:  `https://linters.sssa.aws.intellij.net/v1`,
			success:        true,
		},
		{
			name:           "wrong response",
			cooldown:       0,
			failedAttempts: 0,
			actualAttempts: 1,
			response:       `{"api":}`,
			cloudApiUrl:    `https://api.qodana.cloud/v1`,
			lintersApiUrl:  `https://linters.sssa.aws.intellij.net/v1`,
			success:        false,
		},
		{
			name:           "perfect server several versions",
			cooldown:       0,
			failedAttempts: 0,
			actualAttempts: 1,
			response: `{
   "api":{
      "versions":[
         {
            "version":"1.10",
            "url":"https://api.qodana.cloud/v1"
         },
		{	
            "version":"2.13",
            "url":"https://api.qodana.cloud/v2"
         }
      ]
   },
   "linters":{
      "versions":[
         {
            "version":"1.0",
            "url":"https://linters.sssa.aws.intellij.net/v1"
         },
		{
            "version":"3.1",
            "url":"https://linters.sssa.aws.intellij.net/v3"
         }
      ]
   }
}`,
			cloudApiUrl:   `https://api.qodana.cloud/v1`,
			lintersApiUrl: `https://linters.sssa.aws.intellij.net/v1`,
			success:       true,
		},
	} {
		t.Run(
			testData.name, func(t *testing.T) {
				t.Cleanup(
					func() {
						endpoint = nil
						endpointApis = nil
					},
				)
				cloudEndpoint, apiEndpoints, err := runRequest(
					t,
					testData.cooldown,
					testData.failedAttempts,
					testData.response,
				)
				assert.Equal(t, err == nil, testData.success)
				if testData.success {
					assert.Equal(
						t, *apiEndpoints, QdApiEndpoints{
							RootEndpoint:  cloudEndpoint,
							CloudApiUrl:   testData.cloudApiUrl,
							LintersApiUrl: testData.lintersApiUrl,
						},
					)
				}

			},
		)
	}
}

func TestWrongVersion(t *testing.T) {
	for _, testData := range []struct {
		name       string
		response   string
		errorOnApi string
		versions   []string
	}{
		{
			name:       "wrong major cloud version",
			response:   `{"api":{"versions":[{"version":"2.0","url":"https://api.qodana.cloud/v1"}]},"linters":{"versions":[{"version":"1.0","url":"https://linters.sssa.aws.intellij.net/v1"}]}}`,
			errorOnApi: "cloud",
			versions:   []string{"2.0"},
		},
		{
			name:       "wrong major linters version",
			response:   `{"api":{"versions":[{"version":"1.1","url":"https://api.qodana.cloud/v1"}]},"linters":{"versions":[{"version":"3.11","url":"https://linters.sssa.aws.intellij.net/v1"}]}}`,
			errorOnApi: "linters",
			versions:   []string{"3.11"},
		},
		{
			name: "several wrong major versions",
			response: `
			{
			   "api":{
				  "versions":[
					 {
						"version":"2.10",
						"url":"https://api.qodana.cloud/v1"
					 },
					{	
						"version":"3.13",
						"url":"https://api.qodana.cloud/v2"
					 }
				  ]
			   },
			   "linters":{
				  "versions":[
					 {
						"version":"1.0",
						"url":"https://linters.sssa.aws.intellij.net/v1"
					 },
					{
						"version":"3.1",
						"url":"https://linters.sssa.aws.intellij.net/v3"
					 }
				  ]
			   }
			}`,
			errorOnApi: "cloud",
			versions:   []string{"2.10", "3.13"},
		},
	} {
		t.Run(
			testData.name, func(t *testing.T) {
				t.Cleanup(
					func() {
						endpoint = nil
						endpointApis = nil
					},
				)
				_, _, err := runRequest(t, 0, 0, testData.response)

				var versionError *ApiVersionMismatchError
				res := errors.As(err, &versionError)
				if !res {
					t.Errorf("Should be version error")
				}
				assert.Equal(
					t, *versionError, ApiVersionMismatchError{
						ApiKind:           testData.errorOnApi,
						SupportedVersions: testData.versions,
					},
				)
			},
		)
	}
}

func runRequest(
	t *testing.T,
	cooldown int,
	failedAttempts int,
	response string,
) (*QdRootEndpoint, *QdApiEndpoints, error) {
	requestServed := 0
	svr := httptest.NewTLSServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				requestServed++
				if r.URL.Path != VersionsURI {
					t.Errorf("expected uri to be '%s' got '%s'", VersionsURI, r.URL.Path)
				}
				if requestServed <= failedAttempts {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				_, _ = fmt.Fprint(w, response)
			},
		),
	)
	defer svr.Close()
	client := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	t.Setenv(qdenv.QodanaCloudRequestCooldownEnv, strconv.Itoa(cooldown))
	t.Setenv(qdenv.QodanaEndpointEnv, svr.URL)
	cloudEndpoint := GetCloudRootEndpoint()
	apiEndpoints, err := cloudEndpoint.requestApiEndpointsCustomClient(&client)
	return cloudEndpoint, apiEndpoints, err
}
