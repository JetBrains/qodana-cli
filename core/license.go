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
	"errors"
	"fmt"
	"github.com/JetBrains/qodana-cli/cloud"
	"log"
	"os"
)

func SetupLicense(token string) {
	_, exists := os.LookupEnv(QodanaLicense)
	if exists {
		return
	}

	// community versions works without any license and can't check any license
	if Prod.IsCommunity() {
		return
	}

	// eap version works with eap's license dependent on build date
	if Prod.EAP {
		if token == "" {
			fmt.Println(cloud.EapWarnTokenMessage)
			fmt.Println()
		}
		return
	}

	// usual builds should have token for execution
	if token == "" {
		log.Fatal(cloud.EmptyTokenMessage)
	}

	licenseEndpoint := cloud.GetEnvWithDefault(QodanaLicenseEndpoint, "https://linters.qodana.cloud")

	licenseDataResponse, err := cloud.RequestLicenseData(licenseEndpoint, token)
	if errors.Is(err, cloud.TokenDeclinedError) {
		log.Fatalf("License request: %v\n%s", err, cloud.DeclinedTokenErrorMessage)
	}
	if err != nil {
		log.Fatalf("License request: %v\n%s", err, cloud.GeneralLicenseErrorMessage)
	}
	licenseKey := cloud.ExtractLicenseKey(licenseDataResponse)
	if licenseKey == "" {
		log.Fatalf("Response for license request should contain license key\n%s", string(licenseDataResponse))
	}
	err = os.Setenv(QodanaLicense, licenseKey)
	if err != nil {
		log.Fatal(err)
	}
}

func SetupLicenseToken(opts *QodanaOptions) {
	token := opts.loadToken(false)
	licenseOnlyToken := os.Getenv(QodanaLicenseOnlyToken)

	if token == "" && licenseOnlyToken != "" {
		cloud.Token = cloud.LicenseToken{
			Token:       licenseOnlyToken,
			LicenseOnly: true,
		}
	} else {
		cloud.Token = cloud.LicenseToken{
			Token:       token,
			LicenseOnly: false,
		}
	}
}
