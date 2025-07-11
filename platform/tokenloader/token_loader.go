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

package tokenloader

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2025/cloud"
	"github.com/JetBrains/qodana-cli/v2025/platform/git"
	"github.com/JetBrains/qodana-cli/v2025/platform/msg"
	"github.com/JetBrains/qodana-cli/v2025/platform/product"
	"github.com/JetBrains/qodana-cli/v2025/platform/qdenv"
	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/go-keyring"
	"os"
)

const keyringDefaultService = "qodana-cli"

type CloudTokenLoader interface {
	GetQodanaToken() string

	GetId() string
	GetAnalyzer() product.Analyzer

	GetProjectDir() string
	GetLogDir() string
}

func IsCloudTokenRequired(tokenLoader CloudTokenLoader) bool {
	if tokenLoader.GetQodanaToken() != "" || os.Getenv(qdenv.QodanaLicenseOnlyToken) != "" {
		return true
	}

	isQodanaLicenseSet := os.Getenv(qdenv.QodanaLicense) != ""
	analyzer := tokenLoader.GetAnalyzer()
	isFreeAnalyzer := !analyzer.GetLinter().IsPaid
	isEapAnalyzer := analyzer.IsEAP()

	return !(isQodanaLicenseSet || isFreeAnalyzer || isEapAnalyzer)
}

func LoadCloudUploadToken(tokenLoader CloudTokenLoader, refresh bool, requiresToken bool, interactive bool) string {
	tokenFetchers := []func(bool) string{
		func(_ bool) string { return tokenLoader.GetQodanaToken() },
		func(refresh bool) string { return getTokenFromKeychain(refresh, tokenLoader.GetId()) },
	}
	if interactive && requiresToken {
		fetcherFromUserInput := func(_ bool) string {
			return getTokenFromUserInput(tokenLoader.GetProjectDir(), tokenLoader.GetId(), tokenLoader.GetLogDir())
		}
		tokenFetchers = append(tokenFetchers, fetcherFromUserInput)
	}

	for _, fetcher := range tokenFetchers {
		if token := fetcher(refresh); token != "" {
			return token
		}
	}
	return ""
}

func ValidateCloudToken(tokenLoader CloudTokenLoader, refresh bool) string {
	token := LoadCloudUploadToken(tokenLoader, refresh, true, true)
	if token != "" {
		ValidateTokenPrintProject(token)
	}
	return token
}

// ValidateTokenPrintProject validates given token by requesting linked project name.
func ValidateTokenPrintProject(token string) {
	client := cloud.GetCloudApiEndpoints().NewCloudApiClient(token)
	if projectName, err := client.RequestProjectName(); err != nil {
		msg.ErrorMessage(cloud.InvalidTokenMessage)
		os.Exit(1)
	} else {
		if !qdenv.IsContainer() {
			msg.SuccessMessage("Linked %s project: %s", cloud.GetCloudRootEndpoint().Url, projectName)
		}
	}
}

// saveCloudToken saves token to the system keyring
func saveCloudToken(id string, token string) error {
	err := keyring.Set(keyringDefaultService, id, token)
	if err != nil {
		return err
	}
	log.Debugf("Saved token to the system keyring with id %s", id)
	return nil
}

// getCloudToken returns token from the system keyring
func getCloudToken(id string) (string, error) {
	secret, err := keyring.Get(keyringDefaultService, id)
	if err != nil {
		return "", err
	}
	log.Debugf("Got token from the system keyring with id %s", id)
	return secret, nil
}

func setupToken(path string, id string, logdir string) string {
	openCloud := msg.AskUserConfirm("Do you want to open the team page to get the token?")
	if openCloud {
		origin, err := git.RemoteUrl(path, logdir)
		if err != nil {
			msg.ErrorMessage("%s", err)
			return ""
		}
		err = utils.OpenBrowser(cloud.GetCloudRootEndpoint().GetCloudTeamsPageUrl(origin, path))
		if err != nil {
			msg.ErrorMessage("%s", err)
			return ""
		}
	}
	token, err := pterm.DefaultInteractiveTextInput.WithMask("*").WithTextStyle(msg.PrimaryStyle).Show(
		fmt.Sprintf(">  Enter the token (will be used for %s; enter 'q' to exit)", msg.PrimaryBold(path)),
	)
	if token == "q" {
		return "q"
	}
	if err != nil {
		msg.ErrorMessage("%s", err)
		return ""
	}
	if token == "" {
		msg.ErrorMessage("Token cannot be empty")
		return ""
	} else {
		client := cloud.GetCloudApiEndpoints().NewCloudApiClient(token)
		_, err := client.RequestProjectName()
		if err != nil {
			msg.ErrorMessage("Invalid token, try again")
			return ""
		}
		err = saveCloudToken(id, token)
		if err != nil {
			msg.ErrorMessage("Failed to save credentials: %s", err)
			return ""
		}
		return token
	}
}

func getTokenFromKeychain(refresh bool, id string) string {
	log.Debugf("project id: %s", id)
	if refresh || os.Getenv(qdenv.QodanaClearKeyring) != "" {
		err := keyring.Delete(keyringDefaultService, id)
		if err != nil {
			log.Debugf("Failed to delete token from the system keyring: %s", err)
		}
		return ""
	}
	tokenFromKeychain, err := getCloudToken(id)
	if err == nil && tokenFromKeychain != "" {
		msg.WarningMessage(
			"Got %s from the system keyring, declare %s env variable or run %s to override it",
			msg.PrimaryBold(qdenv.QodanaToken),
			msg.PrimaryBold(qdenv.QodanaToken),
			msg.PrimaryBold("qodana init -f"),
		)
		log.Debugf("Loaded token from the system keyring with id %s", id)
		return tokenFromKeychain
	}
	return ""
}

func getTokenFromUserInput(projectDir string, id string, logDir string) string {
	if msg.IsInteractive() {
		msg.WarningMessage(cloud.EmptyTokenMessage, cloud.GetCloudRootEndpoint().Url)
		var token string
		for {
			token = setupToken(projectDir, id, logDir)
			if token == "q" {
				return ""
			}
			if token != "" {
				return token
			}
		}
	}
	return ""
}
