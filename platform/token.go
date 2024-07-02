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

package platform

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/cloud"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/go-keyring"
	"os"
)

const defaultService = "qodana-cli"

func (o *QodanaOptions) LoadToken(refresh bool, requiresToken bool, interactive bool) string {
	tokenFetchers := []func(bool) string{
		func(_ bool) string { return o.getTokenFromDockerArgs() },
		func(_ bool) string { return o.getTokenFromEnv() },
		o.getTokenFromKeychain,
	}
	if interactive {
		tokenFetchers = append(tokenFetchers, func(_ bool) string { return o.getTokenFromUserInput(requiresToken) })
	}

	for _, fetcher := range tokenFetchers {
		if token := fetcher(refresh); token != "" {
			return token
		}
	}
	return ""
}

func (o *QodanaOptions) getTokenFromDockerArgs() string {
	tokenFromCliArgs := o.Getenv(QodanaToken)
	if tokenFromCliArgs != "" {
		log.Debug("Loaded token from CLI args environment")
		return tokenFromCliArgs
	}
	return ""
}

func (o *QodanaOptions) getTokenFromEnv() string {
	tokenFromEnv := os.Getenv(QodanaToken)
	if tokenFromEnv != "" {
		log.Debug("Loaded token from the environment variable")
		return tokenFromEnv
	}
	return ""
}

func (o *QodanaOptions) getTokenFromKeychain(refresh bool) string {
	log.Debugf("project id: %s", o.Id())
	if refresh || os.Getenv(qodanaClearKeyring) != "" {
		err := keyring.Delete(defaultService, o.Id())
		if err != nil {
			log.Debugf("Failed to delete token from the system keyring: %s", err)
		}
		return ""
	}
	tokenFromKeychain, err := getCloudToken(o.Id())
	if err == nil && tokenFromKeychain != "" {
		WarningMessage(
			"Got %s from the system keyring, declare %s env variable or run %s to override it",
			PrimaryBold(QodanaToken),
			PrimaryBold(QodanaToken),
			PrimaryBold("qodana init -f"),
		)
		o.Setenv(QodanaToken, tokenFromKeychain)
		log.Debugf("Loaded token from the system keyring with id %s", o.Id())
		return tokenFromKeychain
	}
	return ""
}

func (o *QodanaOptions) getTokenFromUserInput(requiresToken bool) string {
	if IsInteractive() && requiresToken {
		WarningMessage(cloud.EmptyTokenMessage, cloud.GetCloudRootEndpoint().GetCloudUrl())
		var token string
		for {
			token = setupToken(o.ProjectDir, o.Id())
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

// ValidateToken checks if QODANA_TOKEN is set in CLI args, or environment or the system keyring, returns its value.
func (o *QodanaOptions) ValidateToken(refresh bool) string {
	token := o.LoadToken(refresh, true, true)
	if token != "" {
		ValidateTokenPrintProject(token)
		o.Setenv(QodanaToken, token)
	}
	return token
}

// ValidateTokenPrintProject validates given token by requesting linked project name.
func ValidateTokenPrintProject(token string) {
	client := cloud.GetCloudApiEndpoints().NewCloudApiClient(token)
	if projectName, err := client.RequestProjectName(); err != nil {
		ErrorMessage(cloud.InvalidTokenMessage)
		os.Exit(1)
	} else {
		if !IsContainer() {
			SuccessMessage("Linked %s project: %s", cloud.GetCloudRootEndpoint().Host, projectName)
		}
	}
}

// saveCloudToken saves token to the system keyring
func saveCloudToken(id string, token string) error {
	err := keyring.Set(defaultService, id, token)
	if err != nil {
		return err
	}
	log.Debugf("Saved token to the system keyring with id %s", id)
	return nil
}

// getCloudToken returns token from the system keyring
func getCloudToken(id string) (string, error) {
	secret, err := keyring.Get(defaultService, id)
	if err != nil {
		return "", err
	}
	log.Debugf("Got token from the system keyring with id %s", id)
	return secret, nil
}

func setupToken(path string, id string) string {
	openCloud := AskUserConfirm("Do you want to open the team page to get the token?")
	if openCloud {
		origin := GitRemoteUrl(path)
		err := openBrowser(cloud.GetCloudRootEndpoint().GetCloudTeamsPageUrl(origin, path))
		if err != nil {
			ErrorMessage("%s", err)
			return ""
		}
	}
	token, err := pterm.DefaultInteractiveTextInput.WithMask("*").WithTextStyle(PrimaryStyle).Show(
		fmt.Sprintf(">  Enter the token (will be used for %s; enter 'q' to exit)", PrimaryBold(path)),
	)
	if token == "q" {
		return "q"
	}
	if err != nil {
		ErrorMessage("%s", err)
		return ""
	}
	if token == "" {
		ErrorMessage("Token cannot be empty")
		return ""
	} else {
		client := cloud.GetCloudApiEndpoints().NewCloudApiClient(token)
		_, err := client.RequestProjectName()
		if err != nil {
			ErrorMessage("Invalid token, try again")
			return ""
		}
		err = saveCloudToken(id, token)
		if err != nil {
			ErrorMessage("Failed to save credentials: %s", err)
			return ""
		}
		return token
	}
}
