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
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var testOptions = &QodanaOptions{
	ResultsDir:            "./results",
	CacheDir:              "./cache",
	ProjectDir:            "./project",
	Linter:                "jetbrains/qodana-jvm-community:latest",
	SourceDirectory:       "./src",
	DisableSanity:         true,
	RunPromo:              "true",
	Baseline:              "qodana.sarif.json",
	BaselineIncludeAbsent: true,
	SaveReport:            true,
	ShowReport:            true,
	Port:                  8888,
	Property:              []string{"foo.baz=bar", "foo.bar=baz"},
	Script:                "default",
	FailThreshold:         "0",
	AnalysisId:            "id",
	Env:                   []string{"A=B"},
	Volumes:               []string{"/tmp/foo:/tmp/foo"},
	User:                  "1001:1001",
	PrintProblems:         true,
	ProfileName:           "Default",
}

// TestScanFlags verify that the option struct is converted to the wanted Qodana Docker options.
func TestScanFlags(t *testing.T) {
	expected := strings.Join([]string{
		"--save-report",
		"--source-directory",
		"./src",
		"--disable-sanity",
		"--profile-name",
		"Default",
		"--run-promo true",
		"--baseline",
		"qodana.sarif.json",
		"--baseline-include-absent",
		"--fail-threshold",
		"0",
		"--analysis-id",
		"id",
		"--property=foo.baz=bar",
		"--property=foo.bar=baz",
	}, " ")
	actual := strings.Join(getCmdOptions(testOptions), " ")
	if expected != actual {
		t.Fatalf("expected \"%s\" got \"%s\"", expected, actual)
	}
}

func TestCloudUrl(t *testing.T) {
	for _, tc := range []struct {
		name        string
		writtenUrl  string
		expectedUrl string
	}{
		{
			name:        "valid url",
			writtenUrl:  "https://youtu.be/dQw4w9WgXcQ",
			expectedUrl: "https://youtu.be/dQw4w9WgXcQ",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resultsPath := filepath.Join(os.TempDir(), "cloud_url_valid")
			err := os.MkdirAll(resultsPath, 0o755)
			if err != nil {
				return
			}

			filePath := resultsPath + "/" + qodanaReportUrlFile
			err = os.WriteFile(
				filePath,
				[]byte(tc.writtenUrl),
				0o644,
			)
			if err != nil {
				t.Fatal(err)
			}
			actual := GetReportUrl(resultsPath)
			if actual != tc.expectedUrl {
				t.Fatalf("expected \"%s\" got \"%s\"", tc.expectedUrl, actual)
			}
		})
	}
}

func Test_ExtractEnvironmentVariables(t *testing.T) {
	revisionExpected := "1234567890abcdef1234567890abcdef12345678"
	branchExpected := "main"

	if os.Getenv("GITHUB_ACTIONS") == "true" {
		variables := []string{
			"GITHUB_SERVER_URL",
			"GITHUB_REPOSITORY",
			"GITHUB_RUN_ID",
			"GITHUB_HEAD_REF",
			"GITHUB_REF",
		}
		for _, v := range variables {
			_ = os.Unsetenv(v)
		}
	}

	for _, tc := range []struct {
		ci                      string
		variables               map[string]string
		qodanaJobUrlExpected    string
		qodanaEnvExpected       string
		qodanaRemoteUrlExpected string
	}{
		{
			ci:                "no CI detected",
			variables:         map[string]string{},
			qodanaEnvExpected: "cli:dev",
		},
		{
			ci: "User defined",
			variables: map[string]string{
				qodanaEnv:       "user-defined",
				qodanaJobUrl:    "https://qodana.jetbrains.com/never-gonna-give-you-up",
				qodanaRemoteUrl: "https://qodana.jetbrains.com/never-gonna-give-you-up",
				qodanaBranch:    branchExpected,
				qodanaRevision:  revisionExpected,
			},
			qodanaEnvExpected:       "user-defined",
			qodanaRemoteUrlExpected: "https://qodana.jetbrains.com/never-gonna-give-you-up",
			qodanaJobUrlExpected:    "https://qodana.jetbrains.com/never-gonna-give-you-up",
		},
		{
			ci: "GitLab",
			variables: map[string]string{
				"CI_JOB_URL":        "https://gitlab.jetbrains.com/never-gonna-give-you-up",
				"CI_COMMIT_BRANCH":  branchExpected,
				"CI_COMMIT_SHA":     revisionExpected,
				"CI_REPOSITORY_URL": "https://gitlab.jetbrains.com/sa/entrypoint.git",
			},
			qodanaEnvExpected:       fmt.Sprintf("gitlab:%s", Version),
			qodanaRemoteUrlExpected: "https://gitlab.jetbrains.com/sa/entrypoint.git",
			qodanaJobUrlExpected:    "https://gitlab.jetbrains.com/never-gonna-give-you-up",
		},
		{
			ci: "Jenkins",
			variables: map[string]string{
				"BUILD_URL":        "https://jenkins.jetbrains.com/never-gonna-give-you-up",
				"GIT_LOCAL_BRANCH": branchExpected,
				"GIT_COMMIT":       revisionExpected,
				"GIT_URL":          "https://git.jetbrains.com/sa/entrypoint.git",
			},
			qodanaEnvExpected:       fmt.Sprintf("jenkins:%s", Version),
			qodanaJobUrlExpected:    "https://jenkins.jetbrains.com/never-gonna-give-you-up",
			qodanaRemoteUrlExpected: "https://git.jetbrains.com/sa/entrypoint.git",
		},
		{
			ci: "GitHub",
			variables: map[string]string{
				"GITHUB_SERVER_URL": "https://github.jetbrains.com",
				"GITHUB_REPOSITORY": "sa/entrypoint",
				"GITHUB_RUN_ID":     "123456789",
				"GITHUB_SHA":        revisionExpected,
				"GITHUB_HEAD_REF":   branchExpected,
			},
			qodanaEnvExpected:       fmt.Sprintf("github-actions:%s", Version),
			qodanaJobUrlExpected:    "https://github.jetbrains.com/sa/entrypoint/actions/runs/123456789",
			qodanaRemoteUrlExpected: "https://github.jetbrains.com/sa/entrypoint.git",
		},
		{
			ci: "CircleCI",
			variables: map[string]string{
				"CIRCLE_BUILD_URL":      "https://circleci.jetbrains.com/never-gonna-give-you-up",
				"CIRCLE_SHA1":           revisionExpected,
				"CIRCLE_BRANCH":         branchExpected,
				"CIRCLE_REPOSITORY_URL": "https://circleci.jetbrains.com/sa/entrypoint.git",
			},
			qodanaEnvExpected:       fmt.Sprintf("circleci:%s", Version),
			qodanaJobUrlExpected:    "https://circleci.jetbrains.com/never-gonna-give-you-up",
			qodanaRemoteUrlExpected: "https://circleci.jetbrains.com/sa/entrypoint.git",
		},
		{
			ci: "Azure Pipelines",
			variables: map[string]string{
				"SYSTEM_TEAMFOUNDATIONCOLLECTIONURI": "https://dev.azure.com/jetbrains",
				"BUILD_BUILDURI":                     "https://dev.azure.com/jetbrains/never-gonna-give-you-up",
				"SYSTEM_TEAMPROJECT":                 "/sa",
				"BUILD_BUILDID":                      "123456789",
				"BUILD_SOURCEVERSION":                revisionExpected,
				"BUILD_SOURCEBRANCH":                 "refs/heads/" + branchExpected,
				"BUILD_REPOSITORY_URI":               "https://dev.azure.com/jetbrains/sa/entrypoint.git",
			},
			qodanaEnvExpected:       fmt.Sprintf("azure-pipelines:%s", Version),
			qodanaJobUrlExpected:    "https://dev.azure.com/jetbrains/sa/_build/results?buildId=123456789",
			qodanaRemoteUrlExpected: "https://dev.azure.com/jetbrains/sa/entrypoint.git",
		},
	} {
		t.Run(tc.ci, func(t *testing.T) {
			opts := &QodanaOptions{}
			for k, v := range tc.variables {
				err := os.Setenv(k, v)
				if err != nil {
					t.Fatal(err)
				}
				opts.setenv(k, v)
			}

			extractQodanaEnvironment(opts)
			currentQodanaEnv := opts.getenv(qodanaEnv)
			if currentQodanaEnv != tc.qodanaEnvExpected {
				t.Errorf("Expected %s, got %s", tc.qodanaEnvExpected, currentQodanaEnv)
			}
			if !strings.HasPrefix(currentQodanaEnv, "cli:") {
				if opts.getenv(qodanaJobUrl) != tc.qodanaJobUrlExpected {
					t.Errorf("Expected %s, got %s", tc.qodanaJobUrlExpected, opts.getenv(qodanaJobUrl))
				}
				if opts.getenv(qodanaRemoteUrl) != tc.qodanaRemoteUrlExpected {
					t.Errorf("Expected %s, got %s", tc.qodanaRemoteUrlExpected, opts.getenv(qodanaRemoteUrl))
				}
				if opts.getenv(qodanaRevision) != revisionExpected {
					t.Errorf("Expected %s, got %s", revisionExpected, opts.getenv(qodanaRevision))
				}
				if opts.getenv(qodanaBranch) != branchExpected {
					t.Errorf("Expected %s, got %s", branchExpected, opts.getenv(qodanaBranch))
				}
			}
			for _, k := range []string{qodanaJobUrl, qodanaEnv, qodanaRemoteUrl, qodanaRevision, qodanaBranch} {
				err := os.Unsetenv(k)
				if err != nil {
					t.Fatal(err)
				}
			}
			for k := range tc.variables {
				err := os.Unsetenv(k)
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestDirLanguagesExcluded(t *testing.T) {
	expected := []string{"Go", "Shell"}
	actual, err := recognizeDirLanguages("../")
	if err != nil {
		return
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("expected \"%s\" got \"%s\"", expected, actual)
	}
}

func TestScanFlags_Script(t *testing.T) {
	testOptions := &QodanaOptions{
		Script: "custom-script:parameters",
	}
	expected := []string{
		"--script",
		"custom-script:parameters",
	}
	actual := getCmdOptions(testOptions)
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("expected \"%s\" got \"%s\"", expected, actual)
	}
}
