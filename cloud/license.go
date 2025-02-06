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
	"github.com/JetBrains/qodana-cli/v2024/platform/qdenv"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

type LicenseData struct {
	LicenseID          string `json:"licenseId"`
	LicenseKey         string `json:"licenseKey"`
	ExpirationDate     string `json:"expirationDate"`
	ProjectIdHash      string `json:"projectIdHash"`
	OrganisationIdHash string `json:"organizationIdHash"`
	LicensePlan        string `json:"licensePlan"`
}

type LicenseToken struct {
	Token string
	// LicenseOnly is true if the token is used only for license verification
	LicenseOnly bool
}

const (
	QodanaLicenseRequestCooldownEnv = "QODANA_LICENSE_REQUEST_COOLDOWN"

	QodanaLicenseRequestTimeoutEnv = "QODANA_LICENSE_REQUEST_TIMEOUT"

	QodanaLicenseRequestAttemptsCountEnv = "QODANA_LICENSE_ATTEMPTS"

	qodanaLicenseRequestAttemptsCount = 3

	qodanaLicenseRequestTimeout = 60

	qodanaLicenseRequestCooldown = 60

	qodanaLicenseUri     = "/linters/license-key"
	CommunityLicensePlan = "COMMUNITY"
)

var TokenDeclinedError = errors.New("token was declined by Qodana Cloud server")

var EmptyTokenMessage = `Starting from version 2023.2 release versions of Qodana Linters require connection to Qodana Cloud. 
To continue using Qodana, please ensure you have an access token and provide the token as the QODANA_TOKEN environment variable.
Obtain your token by registering at %s
For more details, please visit: https://www.jetbrains.com/help/qodana/cloud-quickstart.html
We also offer Community versions as an alternative. You can find them here: https://www.jetbrains.com/help/qodana/linters.html
`

var EapWarnTokenMessage = `
Starting from version 2023.2 release versions of Qodana Linters will require connection to Qodana Cloud. 
For seamless transition to release versions, obtain your token by registering at %s 
and provide the token as the QODANA_TOKEN environment variable.
For more details, please visit: https://www.jetbrains.com/help/qodana/cloud-quickstart.html`

var GeneralLicenseErrorMessage = `
Please check if %s is accessible from your environment. 
If you encounter any issues, please contact us at qodana-support@jetbrains.com. 
Or use our issue tracker at https://jb.gg/qodana-issue`

const InvalidTokenMessage = `QODANA_TOKEN is invalid, please provide a valid token`

const DeclinedTokenErrorMessage = `
License verification failed. Please ensure that the token provided through the QODANA_TOKEN 
environment variable is correct and that you have a valid license. 
If you need further assistance, please contact our support team at qodana-support@jetbrains.com`

var Token LicenseToken

func (o *LicenseToken) IsAllowedToSendReports() bool {
	return !o.LicenseOnly && o.Token != ""
}

func (o *LicenseToken) IsAllowedToSendFUS() bool {
	return !o.LicenseOnly
}

func DeserializeLicenseData(data []byte) LicenseData {
	var ld LicenseData
	err := json.Unmarshal(data, &ld)
	if err != nil {
		log.Fatalf("License deserialization failed. License response data:\n%s\nError: '%v'", string(data), err)
	}
	return ld
}

func (endpoints *QdApiEndpoints) RequestLicenseData(token string) ([]byte, error) {
	attempts := getAttempts()
	cooldown := getCooldown()
	for i := 1; i <= attempts; i++ {
		license, err := requestLicenseDataAttempt(endpoints.LintersApiUrl, token)
		if errors.Is(err, TokenDeclinedError) {
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
	if resp.StatusCode == 401 || resp.StatusCode == 404 {
		return nil, TokenDeclinedError
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
	return GetEnvWithDefaultInt(QodanaLicenseRequestTimeoutEnv, qodanaLicenseRequestTimeout)
}

func getCooldown() int {
	return GetEnvWithDefaultInt(QodanaLicenseRequestCooldownEnv, qodanaLicenseRequestCooldown)
}

func getAttempts() int {
	return GetEnvWithDefaultInt(QodanaLicenseRequestAttemptsCountEnv, qodanaLicenseRequestAttemptsCount)
}

func GetEnvWithDefault(env string, defaultValue string) string {
	value, exists := os.LookupEnv(env)
	if !exists {
		return defaultValue
	}
	return value
}

func GetEnvWithDefaultInt(env string, defaultValue int) int {
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

func SetupLicenseToken(token string) {
	licenseOnlyToken := os.Getenv(qdenv.QodanaLicenseOnlyToken)
	if token == "" && licenseOnlyToken != "" {
		Token = LicenseToken{
			Token:       licenseOnlyToken,
			LicenseOnly: true,
		}
	} else {
		Token = LicenseToken{
			Token:       token,
			LicenseOnly: false,
		}
	}
}

func (endpoints *QdApiEndpoints) GetLicenseData(token string) LicenseData {
	licenseDataResponse, err := endpoints.RequestLicenseData(token)
	if errors.Is(err, TokenDeclinedError) {
		log.Fatalf("License request: %v\n%s", err, DeclinedTokenErrorMessage)
	}
	if err != nil {
		errMessage := fmt.Sprintf(GeneralLicenseErrorMessage, endpoints.RootEndpoint.GetCloudUrl())
		log.Fatalf("License request: %v\n%s", err, errMessage)
	}
	return DeserializeLicenseData(licenseDataResponse)
}

func (endpoints *QdApiEndpoints) GetLicensePlan(token string) string {
	licenseData := endpoints.GetLicenseData(token)
	log.Debug(fmt.Printf("Qodana license plan: %s", licenseData.LicensePlan))
	return licenseData.LicensePlan
}
