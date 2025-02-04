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
	"github.com/JetBrains/qodana-cli/v2024/cloud"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/go-keyring"
	"os"
	"strings"
)

const keyringDefaultService = "qodana-cli"

type CloudTokenLoader interface {
	GetQodanaToken() string
	GetQodanaLicenseOnlyToken() string

	GetId() string
	GetIde() string
	GetLinter() string

	GetProjectDir() string
	GetLogDir() string
}

func IsCloudTokenRequired(tokenLoader CloudTokenLoader, isCommunityOrEap bool) bool {
	if tokenLoader.GetQodanaToken() != "" || tokenLoader.GetQodanaLicenseOnlyToken() != "" {
		return true
	}

	var analyzer string
	if tokenLoader.GetLinter() != "" {
		analyzer = tokenLoader.GetLinter()
	} else if tokenLoader.GetIde() != "" {
		analyzer = tokenLoader.GetIde()
	}

	if os.Getenv(platform.QodanaLicense) != "" ||
		platform.Contains(append(platform.AllSupportedFreeImages, platform.AllSupportedFreeCodes...), analyzer) ||
		strings.Contains(platform.Lower(analyzer), "eap") ||
		isCommunityOrEap {
		return false
	}

	for _, e := range platform.AllSupportedPaidCodes {
		if strings.HasPrefix(platform.Image(e), tokenLoader.GetLinter()) || strings.HasPrefix(e, tokenLoader.GetIde()) {
			return true
		}
	}

	return false
}

func LoadCloudToken(tokenLoader CloudTokenLoader, refresh bool, requiresToken bool, interactive bool) string {
	tokenFetchers := []func(bool) string{
		func(_ bool) string { return tokenLoader.GetQodanaToken() },
		func(_ bool) string { return getTokenFromEnv() },
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

func ValidateToken(tokenLoader CloudTokenLoader, refresh bool) string {
	token := LoadCloudToken(tokenLoader, refresh, true, true)
	if token != "" {
		ValidateTokenPrintProject(token)
	}
	return token
}

// ValidateTokenPrintProject validates given token by requesting linked project name.
func ValidateTokenPrintProject(token string) {
	client := cloud.GetCloudApiEndpoints().NewCloudApiClient(token)
	if projectName, err := client.RequestProjectName(); err != nil {
		platform.ErrorMessage(cloud.InvalidTokenMessage)
		os.Exit(1)
	} else {
		if !platform.IsContainer() {
			platform.SuccessMessage("Linked %s project: %s", cloud.GetCloudRootEndpoint().Host, projectName)
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
	openCloud := platform.AskUserConfirm("Do you want to open the team page to get the token?")
	if openCloud {
		origin, err := platform.GitRemoteUrl(path, logdir)
		if err != nil {
			platform.ErrorMessage("%s", err)
			return ""
		}
		err = platform.OpenBrowser(cloud.GetCloudRootEndpoint().GetCloudTeamsPageUrl(origin, path))
		if err != nil {
			platform.ErrorMessage("%s", err)
			return ""
		}
	}
	token, err := pterm.DefaultInteractiveTextInput.WithMask("*").WithTextStyle(platform.PrimaryStyle).Show(
		fmt.Sprintf(">  Enter the token (will be used for %s; enter 'q' to exit)", platform.PrimaryBold(path)),
	)
	if token == "q" {
		return "q"
	}
	if err != nil {
		platform.ErrorMessage("%s", err)
		return ""
	}
	if token == "" {
		platform.ErrorMessage("Token cannot be empty")
		return ""
	} else {
		client := cloud.GetCloudApiEndpoints().NewCloudApiClient(token)
		_, err := client.RequestProjectName()
		if err != nil {
			platform.ErrorMessage("Invalid token, try again")
			return ""
		}
		err = saveCloudToken(id, token)
		if err != nil {
			platform.ErrorMessage("Failed to save credentials: %s", err)
			return ""
		}
		return token
	}
}

func getTokenFromEnv() string {
	tokenFromEnv := os.Getenv(platform.QodanaToken)
	if tokenFromEnv != "" {
		log.Debug("Loaded token from the environment variable")
		return tokenFromEnv
	}
	return ""
}

func getTokenFromKeychain(refresh bool, id string) string {
	log.Debugf("project id: %s", id)
	if refresh || os.Getenv(platform.QodanaClearKeyring) != "" {
		err := keyring.Delete(keyringDefaultService, id)
		if err != nil {
			log.Debugf("Failed to delete token from the system keyring: %s", err)
		}
		return ""
	}
	tokenFromKeychain, err := getCloudToken(id)
	if err == nil && tokenFromKeychain != "" {
		platform.WarningMessage(
			"Got %s from the system keyring, declare %s env variable or run %s to override it",
			platform.PrimaryBold(platform.QodanaToken),
			platform.PrimaryBold(platform.QodanaToken),
			platform.PrimaryBold("qodana init -f"),
		)
		log.Debugf("Loaded token from the system keyring with id %s", id)
		return tokenFromKeychain
	}
	return ""
}

func getTokenFromUserInput(projectDir string, id string, logDir string) string {
	if platform.IsInteractive() {
		platform.WarningMessage(cloud.EmptyTokenMessage, cloud.GetCloudRootEndpoint().GetCloudUrl())
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
