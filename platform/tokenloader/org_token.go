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
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/JetBrains/qodana-cli/v2025/cloud"
	"github.com/JetBrains/qodana-cli/v2025/platform/qdenv"
	"github.com/JetBrains/qodana-cli/v2025/platform/qdyaml"
	log "github.com/sirupsen/logrus"
)

var projectIdentifierPartPattern = regexp.MustCompile(`^[A-Za-z0-9 ._-]+$`)

// tokenFromExchange is true when QODANA_TOKEN was obtained by exchanging QODANA_ORG_TOKEN.
// Such a token is short-lived, so it is never saved to the keyring.
var tokenFromExchange = false

// projectTokenTtl is the lifetime of a project token obtained via QODANA_ORG_TOKEN.
const projectTokenTtl = 6 * time.Hour

// exchangeOrgToken is a variable to allow replacing the Cloud call in tests.
var exchangeOrgToken = func(orgToken string, projectQualifiedSlug string) (string, error) {
	return cloud.GetCloudApiEndpoints().NewPublicApiClient(orgToken).RequestProjectToken(projectQualifiedSlug, projectTokenTtl)
}

// ValidateProjectIdentifier checks that a project identifier has the form team-slug:project-slug.
func ValidateProjectIdentifier(identifier string) error {
	parts := strings.Split(identifier, ":")
	if len(parts) != 2 {
		return fmt.Errorf(
			"project identifier '%s' must have the form team-slug:project-slug",
			identifier,
		)
	}
	for _, part := range parts {
		if !projectIdentifierPartPattern.MatchString(part) {
			return fmt.Errorf(
				"project identifier '%s' is invalid: team and project slugs must be non-empty and contain only letters, digits, spaces, '-', '.' and '_'",
				identifier,
			)
		}
	}
	return nil
}

// InitializeQodanaGlobalEnv initializes the global env and resolves QODANA_ORG_TOKEN into QODANA_TOKEN if it is set.
func InitializeQodanaGlobalEnv(provider qdenv.EnvProvider, projectDir string, configName string) {
	qdenv.InitializeQodanaGlobalEnv(provider)
	ResolveOrgToken(projectDir, configName)
}

// ResolveOrgToken exchanges QODANA_ORG_TOKEN for a project token and stores it as QODANA_TOKEN in the global env.
// Does nothing if QODANA_ORG_TOKEN is not set. Must be called right after qdenv.InitializeQodanaGlobalEnv.
func ResolveOrgToken(projectDir string, configName string) {
	if err := resolveOrgToken(projectDir, configName); err != nil {
		log.Fatal(err)
	}
}

func resolveOrgToken(projectDir string, configName string) error {
	tokenFromExchange = false
	orgToken := qdenv.GetQodanaGlobalEnv(qdenv.QodanaOrgToken)
	if orgToken == "" {
		if qdenv.GetQodanaGlobalEnv(qdenv.QodanaProject) != "" {
			log.Debugf("%s is set without %s, ignoring it", qdenv.QodanaProject, qdenv.QodanaOrgToken)
		}
		return nil
	}
	if qdenv.GetQodanaGlobalEnv(qdenv.QodanaToken) != "" {
		return fmt.Errorf(
			"both %s and %s are set, please provide only one of them",
			qdenv.QodanaToken,
			qdenv.QodanaOrgToken,
		)
	}

	identifier, source := getProjectIdentifier(projectDir, configName)
	if identifier == "" {
		return fmt.Errorf(
			"%s is set, but no project is specified: set %s or 'project:' in qodana.yaml (team-slug:project-slug)",
			qdenv.QodanaOrgToken,
			qdenv.QodanaProject,
		)
	}
	if err := ValidateProjectIdentifier(identifier); err != nil {
		return fmt.Errorf("%s: %w", source, err)
	}

	projectToken, err := exchangeOrgToken(orgToken, identifier)
	if err != nil {
		if errors.Is(err, cloud.OrgTokenDeclinedError) {
			return fmt.Errorf("%v\n"+cloud.OrgTokenExchangeFailedMessage, err, identifier)
		}
		return fmt.Errorf("token exchange for project '%s' failed: %w", identifier, err)
	}
	qdenv.SetQodanaGlobalEnv(qdenv.QodanaToken, projectToken)
	tokenFromExchange = true
	// child processes (linter, git, bootstrap, publisher) inherit the env: they must never see the org token
	if err := os.Unsetenv(qdenv.QodanaOrgToken); err != nil {
		return err
	}
	log.Debugf("Obtained a project token for '%s' using %s", identifier, qdenv.QodanaOrgToken)
	return nil
}

// getProjectIdentifier returns the project identifier and where it came from: env takes precedence over qodana.yaml.
func getProjectIdentifier(projectDir string, configName string) (identifier string, source string) {
	if identifier = qdenv.GetQodanaGlobalEnv(qdenv.QodanaProject); identifier != "" {
		return identifier, qdenv.QodanaProject
	}
	yamlPath := qdyaml.GetLocalNotEffectiveQodanaYamlFullPath(projectDir, configName)
	if yamlPath == "" {
		return "", ""
	}
	return qdyaml.LoadQodanaYamlByFullPath(yamlPath).Project, yamlPath
}
