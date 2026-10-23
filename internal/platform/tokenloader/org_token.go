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

	"github.com/JetBrains/qodana-cli/internal/cloud"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	log "github.com/sirupsen/logrus"
)

// projectSlugPartPattern matches the team or the project part of a project slug.
// All allowed characters are ASCII, so the length is counted in characters.
var projectSlugPartPattern = regexp.MustCompile(`^[A-Za-z0-9 ._-]{3,64}$`)

// projectTokenTtl is the lifetime of a project token obtained via QODANA_ORG_TOKEN.
const projectTokenTtl = 6 * time.Hour

// exchangeOrgToken is a variable to allow replacing the Cloud call in tests.
var exchangeOrgToken = func(orgToken string, projectQualifiedSlug string) (string, error) {
	return cloud.GetCloudApiEndpoints().NewPublicApiClient(orgToken).RequestProjectToken(projectQualifiedSlug, projectTokenTtl)
}

// validateProjectSlug checks that a project slug has the form team-slug:project-slug.
func validateProjectSlug(projectSlug string) error {
	parts := strings.Split(projectSlug, ":")
	if len(parts) != 2 {
		return fmt.Errorf(
			"project slug '%s' must have the form team-slug:project-slug",
			projectSlug,
		)
	}
	for _, part := range parts {
		if !projectSlugPartPattern.MatchString(part) {
			return fmt.Errorf(
				"project slug '%s' is invalid: team and project slugs must be 3 to 64 characters long and contain only letters, digits, spaces, '-', '.' and '_'",
				projectSlug,
			)
		}
	}
	return nil
}

// InitializeQodanaGlobalEnv initializes the global env and resolves QODANA_ORG_TOKEN into QODANA_TOKEN if it is set.
// Use it only for commands that talk to Qodana Cloud: the exchange requires a connection.
func InitializeQodanaGlobalEnv(provider qdenv.EnvProvider) {
	qdenv.InitializeQodanaGlobalEnv(provider)
	if err := resolveOrgToken(); err != nil {
		log.Fatal(err)
	}
}

// resolveOrgToken exchanges QODANA_ORG_TOKEN for a project token and stores it as QODANA_TOKEN in the global env.
// Does nothing if QODANA_ORG_TOKEN is not set.
func resolveOrgToken() error {
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

	projectSlug := qdenv.GetQodanaGlobalEnv(qdenv.QodanaProject)
	if projectSlug == "" {
		return fmt.Errorf(
			"%s is set, but no project is specified: set %s (team-slug:project-slug)",
			qdenv.QodanaOrgToken,
			qdenv.QodanaProject,
		)
	}
	if err := validateProjectSlug(projectSlug); err != nil {
		return fmt.Errorf("%s: %w", qdenv.QodanaProject, err)
	}

	projectToken, err := exchangeOrgToken(orgToken, projectSlug)
	if err != nil {
		if errors.Is(err, cloud.ErrOrgTokenDeclined) {
			return fmt.Errorf("%w\n%s", err, fmt.Sprintf(cloud.OrgTokenExchangeFailedMessage, projectSlug))
		}
		return fmt.Errorf("token exchange for project '%s' failed: %w", projectSlug, err)
	}
	qdenv.SetQodanaGlobalEnv(qdenv.QodanaToken, projectToken)
	// child processes (linter, git, bootstrap, publisher) inherit the env: they must never see the org token
	if err := os.Unsetenv(qdenv.QodanaOrgToken); err != nil {
		return err
	}
	log.Debugf("Obtained a project token for '%s' using %s", projectSlug, qdenv.QodanaOrgToken)
	return nil
}

// isTokenFromExchange reports whether QODANA_TOKEN was obtained by exchanging QODANA_ORG_TOKEN.
// Such a token is short-lived, so it is never saved to the keyring.
// Both tokens can't be set at once: resolveOrgToken fails in that case.
func isTokenFromExchange() bool {
	return qdenv.GetQodanaGlobalEnv(qdenv.QodanaOrgToken) != ""
}
