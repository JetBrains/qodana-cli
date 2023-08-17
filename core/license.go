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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

type licenseData struct {
	LicenseID      string `json:"licenseId"`
	LicenseKey     string `json:"licenseKey"`
	ExpirationDate string `json:"expirationDate"`
	LicensePlan    string `json:"licensePlan"`
}

type LicenseToken struct {
	Token       string
	LicenseOnly bool
}

const qodanaLicenseRequestAttemptsCountEnv = "QODANA_LICENSE_ATTEMPTS"

const qodanaLicenseRequestAttemptsCount = 3

const qodanaLicenseRequestTimeoutEnv = "QODANA_LICENSE_REQUEST_TIMEOUT"

const qodanaLicenseRequestTimeout = 60

const qodanaLicenseRequestCooldownEnv = "QODANA_LICENSE_REQUEST_COOLDOWN"

const qodanaLicenseRequestCooldown = 60

const qodanaLicenseEndpoint = "LICENSE_ENDPOINT"

const qodanaLicense = "QODANA_LICENSE"

const qodanaTreatAsRelease = "QODANA_TREAT_AS_RELEASE"

const qodanaLicenseUri = "/v1/linters/license-key"

var tokenDeclinedError = errors.New("token was declined by Qodana Cloud server")

const emptyTokenMessage = `Starting from version 2023.2 release versions of Qodana Linters require connection to Qodana Cloud. 
To continue using Qodana, please ensure you have an access token and provide the token as the QODANA_TOKEN environment variable.
Obtain your token by registering at https://qodana.cloud/
For more details, please visit: https://www.jetbrains.com/help/qodana/cloud-quickstart.html
We also offer Community versions as an alternative. You can find them here: https://www.jetbrains.com/help/qodana/linters.html
`

const eapWarnTokenMessage = `
Starting from version 2023.2 release versions of Qodana Linters will require connection to Qodana Cloud. 
For seamless transition to release versions, obtain your token by registering at https://qodana.cloud/ 
and provide the token as the QODANA_TOKEN environment variable.
For more details, please visit: https://www.jetbrains.com/help/qodana/cloud-quickstart.html`

const generalLicenseErrorMessage = `
Please check if https://qodana.cloud/ is accessible from your environment. 
If you encounter any issues, please contact us at qodana-support@jetbrains.com. 
Or use our issue tracker at https://jb.gg/qodana-issue`

const declinedTokenErrorMessage = `
License verification failed. Please ensure that the token provided through the QODANA_TOKEN 
environment variable is correct and that you have a valid license. 
If you need further assistance, please contact our support team at qodana-support@jetbrains.com`

var licenseToken LicenseToken

func setupLicenseToken(opts *QodanaOptions) {
	token := opts.ValidateToken(false)
	licenseOnlyToken := os.Getenv(qodanaLicenseOnlyToken)

	if token == "" && licenseOnlyToken != "" {
		licenseToken = LicenseToken{
			Token:       licenseOnlyToken,
			LicenseOnly: true,
		}
	} else {
		licenseToken = LicenseToken{
			Token:       token,
			LicenseOnly: false,
		}
	}
}

func (o *LicenseToken) isAllowedToSendReports() bool {
	return !o.LicenseOnly && o.Token != ""
}

func (o *LicenseToken) isAllowedToSendFUS() bool {
	return !o.LicenseOnly
}

func setupLicense(token string) {
	_, exists := os.LookupEnv(qodanaLicense)
	if exists {
		return
	}

	// community versions works without any license and can't check any license
	if Prod.isCommunity() {
		return
	}

	// eap version works with eap's license dependent on build date
	if Prod.EAP {
		if token == "" {
			fmt.Println(eapWarnTokenMessage)
			fmt.Println()
		}
		return
	}

	// usual builds should have token for execution
	if token == "" {
		log.Fatal(emptyTokenMessage)
	}

	licenseEndpoint := getEnv(qodanaLicenseEndpoint, "https://linters.qodana.cloud")

	licenseDataResponse, err := requestLicenseData(licenseEndpoint, token)
	if errors.Is(err, tokenDeclinedError) {
		log.Fatalf("License request: %v\n%s", err, declinedTokenErrorMessage)
	}
	if err != nil {
		log.Fatalf("License request: %v\n%s", err, generalLicenseErrorMessage)
	}
	licenseKey := extractLicenseKey(licenseDataResponse)
	if licenseKey == "" {
		log.Fatalf("Response for license request should contain license key\n%s", string(licenseDataResponse))
	}
	err = os.Setenv(qodanaLicense, licenseKey)
	if err != nil {
		log.Fatal(err)
	}
}

func getEnv(env string, defaultValue string) string {
	value, exists := os.LookupEnv(env)
	if !exists {
		return defaultValue
	}
	return value
}

func getEnvInt(env string, defaultValue int) int {
	value, exists := os.LookupEnv(env)
	if !exists {
		return defaultValue
	}
	result, err := strconv.Atoi(value)
	if err != nil {
		log.Fatalf("Variable '%s' should has integer value but it has value '%s'", env, value)
	}
	return result
}

func extractLicenseKey(data []byte) string {
	var licenseData licenseData
	err := json.Unmarshal(data, &licenseData)
	if err != nil {
		log.Fatalf("License deserialization failed. License response data:\n%s\nError: '%v'", string(data), err)
	}
	return licenseData.LicenseKey
}

func requestLicenseData(endpoint string, token string) ([]byte, error) {
	attempts := getAttempts()
	cooldown := getCooldown()
	for i := 1; i <= attempts; i++ {
		license, err := requestLicenseDataAttempt(endpoint, token)
		if errors.Is(err, tokenDeclinedError) {
			return nil, err
		}
		if err != nil {
			log.Printf(
				"%v\nLicense obtaining attempt %d of %d failed.",
				err,
				i,
				attempts,
			)
			if i < attempts {
				log.Printf("Next attempt in %d seconds", cooldown)
				time.Sleep(time.Duration(cooldown) * time.Second)
			}
		} else {
			return license, nil
		}
	}
	return nil, errors.New("failed to get proper response from Qodana Cloud server")
}

func requestLicenseDataAttempt(endpoint string, token string) ([]byte, error) {
	timeout := getTimeout()

	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}

	url := fmt.Sprintf("%s%s", endpoint, qodanaLicenseUri)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("License request failed\n. %w", err)
	}
	authHeaderValue := fmt.Sprintf("Bearer %s", token)

	req.Header.Set("Authorization", authHeaderValue)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("License request failed\n. %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	bodyText, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Reading license response failed\n. %w", err)
	}
	if resp.StatusCode == 403 {
		return nil, tokenDeclinedError
	}
	if resp.StatusCode == 200 {
		return bodyText, nil
	}
	return nil, fmt.Errorf(
		"License request failed. Response code: %d\nLicense response data:\n%s",
		resp.StatusCode,
		string(bodyText),
	)
}

func getTimeout() int {
	return getEnvInt(qodanaLicenseRequestTimeoutEnv, qodanaLicenseRequestTimeout)
}

func getCooldown() int {
	return getEnvInt(qodanaLicenseRequestCooldownEnv, qodanaLicenseRequestCooldown)
}

func getAttempts() int {
	return getEnvInt(qodanaLicenseRequestAttemptsCountEnv, qodanaLicenseRequestAttemptsCount)
}
