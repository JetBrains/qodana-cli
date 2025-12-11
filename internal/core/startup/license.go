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

package startup

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/JetBrains/qodana-cli/internal/cloud"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
)

func SetupLicenseAndProjectHash(prod product.Product, endpoints *cloud.QdApiEndpoints, token string) {
	var licenseData cloud.LicenseData
	if token != "" {
		licenseData = endpoints.GetLicenseData(token)
		if licenseData.ProjectIdHash != "" {
			err := os.Setenv(qdenv.QodanaProjectIdHash, licenseData.ProjectIdHash)
			if err != nil {
				log.Fatal(err)
			}
		}
		if licenseData.OrganisationIdHash != "" {
			err := os.Setenv(qdenv.QodanaOrganisationIdHash, licenseData.OrganisationIdHash)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
	_, exists := os.LookupEnv(qdenv.QodanaLicense)
	if exists {
		return
	}

	// community versions works without any license and can't check any license
	if !prod.Analyzer.GetLinter().IsPaid {
		return
	}

	// eap version works with eap's license dependent on build date
	if prod.IsEap {
		if token == "" {
			fmt.Printf(cloud.EapWarnTokenMessage, endpoints.RootEndpoint.Url)
			fmt.Println()
			fmt.Println()
		}
		return
	}

	// usual builds should have token and LicenseData for execution
	if token == "" {
		log.Fatalf(cloud.EmptyTokenMessage, endpoints.RootEndpoint.Url)
	}

	licenseDataResponse, err := endpoints.RequestLicenseData(token)
	if errors.Is(err, cloud.ErrTokenDeclined) {
		log.Fatalf("License request: %v\n%s", err, cloud.DeclinedTokenErrorMessage)
	}
	if err != nil {
		errMessage := fmt.Sprintf(cloud.GeneralLicenseErrorMessage, endpoints.RootEndpoint.Url)
		log.Fatalf("License request: %v\n%s", err, errMessage)
	}
	licenseData = cloud.DeserializeLicenseData(licenseDataResponse)
	if strings.ToLower(licenseData.LicensePlan) == "community" {
		log.Fatalf(
			"Your Qodana Cloud organization has Community license that doesn’t support \"%s\" linter, "+
				"please try one of the community linters instead: %s or obtain Ultimate "+
				"or Ultimate Plus license. Read more about licenses and plans at "+
				"https://www.jetbrains.com/help/qodana/pricing.html#pricing-linters-licenses.",
			prod.GetProductNameFromCode(),
			allCommunityNames(),
		)
	}
	if licenseData.LicenseKey == "" {
		log.Fatalf("License key should not be empty\n")
	}
	err = os.Setenv(qdenv.QodanaLicense, licenseData.LicenseKey)
	if err != nil {
		log.Fatal(err)
	}
}

func allCommunityNames() string {
	var nameList []string
	for _, linter := range product.AllSupportedFreeLinters {
		nameList = append(nameList, "\""+linter.PresentableName+"\"")
	}
	return strings.Join(nameList, ", ")
}
