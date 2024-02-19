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

package core

import (
	"errors"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/cloud"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"log"
	"os"
	"strings"
)

func requestLicenseData(token string) cloud.LicenseData {
	licenseEndpoint := cloud.GetEnvWithDefault(platform.QodanaLicenseEndpoint, "https://linters.qodana.cloud")

	licenseDataResponse, err := cloud.RequestLicenseData(licenseEndpoint, token)
	if errors.Is(err, cloud.TokenDeclinedError) {
		log.Fatalf("License request: %v\n%s", err, cloud.DeclinedTokenErrorMessage)
	}
	if err != nil {
		log.Fatalf("License request: %v\n%s", err, cloud.GeneralLicenseErrorMessage)
	}
	return cloud.DeserializeLicenseData(licenseDataResponse)
}

func SetupLicenseAndProjectHash(token string) {
	var licenseData cloud.LicenseData
	if token != "" {
		licenseData = requestLicenseData(token)
		if licenseData.ProjectIdHash != "" {
			err := os.Setenv(platform.QodanaProjectIdHash, licenseData.ProjectIdHash)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
	_, exists := os.LookupEnv(platform.QodanaLicense)
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

	// usual builds should have token and LicenseData for execution
	if token == "" {
		log.Fatal(cloud.EmptyTokenMessage)
	}

	licenseEndpoint := cloud.GetEnvWithDefault(platform.QodanaLicenseEndpoint, "https://linters.qodana.cloud")

	licenseDataResponse, err := cloud.RequestLicenseData(licenseEndpoint, token)
	if errors.Is(err, cloud.TokenDeclinedError) {
		log.Fatalf("License request: %v\n%s", err, cloud.DeclinedTokenErrorMessage)
	}
	if err != nil {
		log.Fatalf("License request: %v\n%s", err, cloud.GeneralLicenseErrorMessage)
	}
	licenseData = cloud.DeserializeLicenseData(licenseDataResponse)
	if strings.ToLower(licenseData.LicensePlan) == "community" {
		log.Fatalf("Your Qodana Cloud organization has Community license that doesn’t support \"%s\" linter, "+
			"please try one of the community linters instead: %s or obtain Ultimate "+
			"or Ultimate Plus license. Read more about licenses and plans at "+
			"https://www.jetbrains.com/help/qodana/pricing.html#pricing-linters-licenses.",
			Prod.getProductNameFromCode(),
			allCommunityNames(),
		)
	}
	if licenseData.LicenseKey == "" {
		log.Fatalf("License key should not be empty\n")
	}
	err = os.Setenv(platform.QodanaLicense, licenseData.LicenseKey)
	if err != nil {
		log.Fatal(err)
	}
}

func allCommunityNames() string {
	var nameList []string
	for _, code := range platform.AllSupportedFreeCodes {
		nameList = append(nameList, "\""+getProductNameFromCode(code)+"\"")
	}
	return strings.Join(nameList, ", ")
}
