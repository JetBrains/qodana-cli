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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JetBrains/qodana-cli/v2025/cloud"
	"github.com/JetBrains/qodana-cli/v2025/platform/product"
	"github.com/JetBrains/qodana-cli/v2025/platform/qdenv"
)

func TestValidateProjectSlug(t *testing.T) {
	for _, testData := range []struct {
		identifier string
		valid      bool
	}{
		{identifier: "a-b:c.d_e", valid: true},
		{identifier: "My Team:My Project 2", valid: true},
		{identifier: " team : project ", valid: true},
		{identifier: "team"},
		{identifier: "team:project:extra"},
		{identifier: ":project"},
		{identifier: "team:"},
		{identifier: ":"},
		{identifier: ""},
		{identifier: "team/x:project"},
		{identifier: "team:pro@ject"},
		{identifier: "tëam:project"},
		{identifier: "team:project\n"},
	} {
		t.Run(
			testData.identifier, func(t *testing.T) {
				err := ValidateProjectSlug(testData.identifier)
				if testData.valid && err != nil {
					t.Errorf("expected '%s' to be valid, got %v", testData.identifier, err)
				}
				if !testData.valid && err == nil {
					t.Errorf("expected '%s' to be invalid", testData.identifier)
				}
			},
		)
	}
}

type envProvider []string

func (e envProvider) Env() []string { return e }

type exchangeCall struct {
	orgToken string
	slug     string
}

func setupOrgTokenTest(t *testing.T, yaml string, env ...string) (string, *[]exchangeCall) {
	t.Helper()
	for _, key := range []string{qdenv.QodanaToken, qdenv.QodanaOrgToken, qdenv.QodanaProjectSlug} {
		t.Setenv(key, "")
	}
	projectDir := t.TempDir()
	if yaml != "" {
		if err := os.WriteFile(filepath.Join(projectDir, "qodana.yaml"), []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	qdenv.InitializeQodanaGlobalEnv(envProvider(env))

	var calls []exchangeCall
	original := exchangeOrgToken
	exchangeOrgToken = func(orgToken string, projectQualifiedSlug string) (string, error) {
		calls = append(calls, exchangeCall{orgToken, projectQualifiedSlug})
		if orgToken == "declined" {
			return "", cloud.OrgTokenDeclinedError
		}
		return "project-token", nil
	}
	t.Cleanup(
		func() {
			exchangeOrgToken = original
			tokenFromExchange = false
		},
	)
	return projectDir, &calls
}

func TestResolveOrgToken(t *testing.T) {
	for _, testData := range []struct {
		name          string
		yaml          string
		env           []string
		expectedCall  *exchangeCall
		expectedToken string
		errorContains string
	}{
		{
			name:          "project token only",
			env:           []string{"QODANA_TOKEN=token"},
			expectedToken: "token",
		},
		{
			name:          "project without org token is ignored",
			env:           []string{"QODANA_TOKEN=token", "QODANA_PROJECT_SLUG=team:project"},
			expectedToken: "token",
		},
		{
			name:          "org token and project from env",
			env:           []string{"QODANA_ORG_TOKEN=org", "QODANA_PROJECT_SLUG=team:project"},
			expectedCall:  &exchangeCall{"org", "team:project"},
			expectedToken: "project-token",
		},
		{
			name:          "org token and project from yaml",
			yaml:          "version: \"1.0\"\nprojectSlug: My Team:my-project\n",
			env:           []string{"QODANA_ORG_TOKEN=org"},
			expectedCall:  &exchangeCall{"org", "My Team:my-project"},
			expectedToken: "project-token",
		},
		{
			name:          "env project overrides yaml",
			yaml:          "projectSlug: yaml-team:yaml-project\n",
			env:           []string{"QODANA_ORG_TOKEN=org", "QODANA_PROJECT_SLUG=env-team:env-project"},
			expectedCall:  &exchangeCall{"org", "env-team:env-project"},
			expectedToken: "project-token",
		},
		{
			name:          "similarly named env is not taken as project slug",
			env:           []string{"QODANA_ORG_TOKEN=org", "QODANA_PROJECT_ID=report-prefix"},
			errorContains: "no project is specified",
		},
		{
			name:          "both tokens",
			env:           []string{"QODANA_TOKEN=token", "QODANA_ORG_TOKEN=org", "QODANA_PROJECT_SLUG=team:project"},
			errorContains: "only one of them",
		},
		{
			name:          "org token without project",
			env:           []string{"QODANA_ORG_TOKEN=org"},
			errorContains: "no project is specified",
		},
		{
			name:          "invalid project from env",
			env:           []string{"QODANA_ORG_TOKEN=org", "QODANA_PROJECT_SLUG=team/project"},
			errorContains: "QODANA_PROJECT_SLUG: project slug",
		},
		{
			name:          "invalid project from yaml",
			yaml:          "projectSlug: team:pro@ject\n",
			env:           []string{"QODANA_ORG_TOKEN=org"},
			errorContains: "qodana.yaml: project slug",
		},
		{
			name:          "declined org token",
			env:           []string{"QODANA_ORG_TOKEN=declined", "QODANA_PROJECT_SLUG=team:project"},
			expectedCall:  &exchangeCall{"declined", "team:project"},
			errorContains: "Failed to obtain a project token",
		},
	} {
		t.Run(
			testData.name, func(t *testing.T) {
				projectDir, calls := setupOrgTokenTest(t, testData.yaml, testData.env...)

				err := resolveOrgToken(projectDir, "")

				if testData.expectedCall == nil && len(*calls) != 0 {
					t.Errorf("expected no exchange, got %+v", *calls)
				}
				if testData.expectedCall != nil && (len(*calls) != 1 || (*calls)[0] != *testData.expectedCall) {
					t.Errorf("expected exchange %+v, got %+v", *testData.expectedCall, *calls)
				}
				if testData.errorContains != "" {
					if err == nil || !strings.Contains(err.Error(), testData.errorContains) {
						t.Fatalf("expected error containing '%s', got %v", testData.errorContains, err)
					}
					return
				}
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if token := qdenv.GetQodanaGlobalEnv(qdenv.QodanaToken); token != testData.expectedToken {
					t.Errorf("expected QODANA_TOKEN '%s', got '%s'", testData.expectedToken, token)
				}
				if tokenFromExchange != (testData.expectedCall != nil) {
					t.Errorf("unexpected tokenFromExchange %v", tokenFromExchange)
				}
			},
		)
	}
}

func TestResolveOrgTokenUnsetsOsEnv(t *testing.T) {
	projectDir, _ := setupOrgTokenTest(t, "")
	t.Setenv(qdenv.QodanaOrgToken, "org")
	t.Setenv(qdenv.QodanaProjectSlug, "team:project")
	qdenv.InitializeQodanaGlobalEnv(qdenv.EmptyEnvProvider())

	if err := resolveOrgToken(projectDir, ""); err != nil {
		t.Fatal(err)
	}
	if _, exists := os.LookupEnv(qdenv.QodanaOrgToken); exists {
		t.Errorf("%s must be removed from the process env", qdenv.QodanaOrgToken)
	}
}

type fakeTokenLoader struct {
	token string
}

func (f fakeTokenLoader) GetQodanaToken() string        { return f.token }
func (f fakeTokenLoader) GetId() string                 { return "test-id" }
func (f fakeTokenLoader) GetAnalyzer() product.Analyzer { return nil }
func (f fakeTokenLoader) GetProjectDir() string         { return "" }
func (f fakeTokenLoader) GetLogDir() string             { return "" }

func TestLoadCloudUploadTokenSkipsKeyringForExchangedToken(t *testing.T) {
	tokenFromExchange = true
	t.Cleanup(func() { tokenFromExchange = false })

	// refresh=true would delete the keyring entry: it must not be reached for an exchanged token
	if token := LoadCloudUploadToken(fakeTokenLoader{token: "project-token"}, true, true, true); token != "project-token" {
		t.Errorf("expected 'project-token', got '%s'", token)
	}
}
