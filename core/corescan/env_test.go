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

package corescan

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdenv"
	"github.com/JetBrains/qodana-cli/v2024/platform/version"
	"golang.org/x/exp/maps"
	"os"
	"testing"
)

func unsetTeamcityVariables() {
	variables := []string{
		"BUILD_VCS_NUMBER",
		"BUILD_NUMBER",
		"TEAMCITY_VERSION",
		"TEAMCITY_BUILDCONF_NAME",
		"BUILD_VCS_URL",
		"BUILD_URL",
		"BUILD_NUMBER",
		"GIT_URL",
		"GIT_COMMIT",
		"GIT_LOCAL_BRANCH",
	}
	for _, v := range variables {
		_ = os.Unsetenv(v)
	}
}

func unsetGitHubVariables() {
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

func Test_ExtractEnvironmentVariables(t *testing.T) {
	revisionExpected := "1234567890abcdef1234567890abcdef12345678"
	branchExpected := "main"

	if os.Getenv("GITHUB_ACTIONS") == "true" {
		unsetGitHubVariables()
	}

	if os.Getenv("TEAMCITY_VERSION") != "" {
		unsetTeamcityVariables()
	}

	for _, tc := range []struct {
		ci                string
		variables         map[string]string
		jobUrlExpected    string
		envExpected       string
		remoteUrlExpected string
		revisionExpected  string
		branchExpected    string
	}{
		{
			ci:          "no CI detected",
			variables:   map[string]string{},
			envExpected: "cli:dev",
		},
		{
			ci: "User defined",
			variables: map[string]string{
				qdenv.QodanaEnv:       "user-defined",
				qdenv.QodanaJobUrl:    "https://qodana.jetbrains.com/never-gonna-give-you-up",
				qdenv.QodanaRemoteUrl: "https://qodana.jetbrains.com/never-gonna-give-you-up",
				qdenv.QodanaBranch:    branchExpected,
				qdenv.QodanaRevision:  revisionExpected,
			},
			envExpected:       "user-defined",
			remoteUrlExpected: "https://qodana.jetbrains.com/never-gonna-give-you-up",
			jobUrlExpected:    "https://qodana.jetbrains.com/never-gonna-give-you-up",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "Space",
			variables: map[string]string{
				"JB_SPACE_EXECUTION_URL":       "https://space.jetbrains.com/never-gonna-give-you-up",
				"JB_SPACE_GIT_BRANCH":          branchExpected,
				"JB_SPACE_GIT_REVISION":        revisionExpected,
				"JB_SPACE_API_URL":             "jetbrains.team",
				"JB_SPACE_PROJECT_KEY":         "sa",
				"JB_SPACE_GIT_REPOSITORY_NAME": "entrypoint",
			},
			envExpected:       fmt.Sprintf("space:%s", version.Version),
			remoteUrlExpected: "ssh://git@git.jetbrains.team/sa/entrypoint.git",
			jobUrlExpected:    "https://space.jetbrains.com/never-gonna-give-you-up",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "GitLab",
			variables: map[string]string{
				"CI_JOB_URL":        "https://gitlab.jetbrains.com/never-gonna-give-you-up",
				"CI_COMMIT_BRANCH":  branchExpected,
				"CI_COMMIT_SHA":     revisionExpected,
				"CI_REPOSITORY_URL": "https://gitlab.jetbrains.com/sa/entrypoint.git",
			},
			envExpected:       fmt.Sprintf("gitlab:%s", version.Version),
			remoteUrlExpected: "https://gitlab.jetbrains.com/sa/entrypoint.git",
			jobUrlExpected:    "https://gitlab.jetbrains.com/never-gonna-give-you-up",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "Jenkins",
			variables: map[string]string{
				"BUILD_URL":        "https://jenkins.jetbrains.com/never-gonna-give-you-up",
				"GIT_LOCAL_BRANCH": branchExpected,
				"GIT_COMMIT":       revisionExpected,
				"GIT_URL":          "https://git.jetbrains.com/sa/entrypoint.git",
			},
			envExpected:       fmt.Sprintf("jenkins:%s", version.Version),
			jobUrlExpected:    "https://jenkins.jetbrains.com/never-gonna-give-you-up",
			remoteUrlExpected: "https://git.jetbrains.com/sa/entrypoint.git",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
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
			envExpected:       fmt.Sprintf("github-actions:%s", version.Version),
			jobUrlExpected:    "https://github.jetbrains.com/sa/entrypoint/actions/runs/123456789",
			remoteUrlExpected: "https://github.jetbrains.com/sa/entrypoint.git",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "GitHub push",
			variables: map[string]string{
				"GITHUB_SERVER_URL": "https://github.jetbrains.com",
				"GITHUB_REPOSITORY": "sa/entrypoint",
				"GITHUB_RUN_ID":     "123456789",
				"GITHUB_SHA":        revisionExpected,
				"GITHUB_REF_NAME":   branchExpected,
			},
			envExpected:       fmt.Sprintf("github-actions:%s", version.Version),
			jobUrlExpected:    "https://github.jetbrains.com/sa/entrypoint/actions/runs/123456789",
			remoteUrlExpected: "https://github.jetbrains.com/sa/entrypoint.git",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "CircleCI",
			variables: map[string]string{
				"CIRCLE_BUILD_URL":      "https://circleci.jetbrains.com/never-gonna-give-you-up",
				"CIRCLE_SHA1":           revisionExpected,
				"CIRCLE_BRANCH":         branchExpected,
				"CIRCLE_REPOSITORY_URL": "https://circleci.jetbrains.com/sa/entrypoint.git",
			},
			envExpected:       fmt.Sprintf("circleci:%s", version.Version),
			jobUrlExpected:    "https://circleci.jetbrains.com/never-gonna-give-you-up",
			remoteUrlExpected: "https://circleci.jetbrains.com/sa/entrypoint.git",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
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
			envExpected:       fmt.Sprintf("azure-pipelines:%s", version.Version),
			jobUrlExpected:    "https://dev.azure.com/jetbrains/sa/_build/results?buildId=123456789",
			remoteUrlExpected: "https://dev.azure.com/jetbrains/sa/entrypoint.git",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "BitBucket Pipelines",
			variables: map[string]string{
				"BITBUCKET_PIPELINE_UUID":  "123456789",
				"BITBUCKET_BUILD_NUMBER":   "123456789",
				"BITBUCKET_REPO_FULL_NAME": "sa/entrypoint",
			},
			envExpected:    fmt.Sprintf("bitbucket:%s", version.Version),
			jobUrlExpected: "https://bitbucket.org/sa/entrypoint/pipelines/results/123456789",
		},
	} {
		t.Run(
			tc.ci, func(t *testing.T) {
				c := Context{}
				for k, v := range tc.variables {
					err := os.Setenv(k, v)
					if err != nil {
						t.Fatal(err)
					}
					c = c.withEnv(k, v, false)
				}

				for _, environment := range []struct {
					name  string
					set   func(string, string)
					unset func(string)
					get   func(string) string
				}{
					{
						name: "Container",
						set:  func(k string, v string) { c = c.withEnv(k, v, false) },
						get:  func(k string) string { return platform.GetEnv(c, k) },
					},
					{
						name: "Local",
						set:  qdenv.SetEnv,
						get:  os.Getenv,
					},
				} {
					t.Run(
						environment.name, func(t *testing.T) {
							qdenv.ExtractQodanaEnvironment(environment.set)
							currentQodanaEnv := environment.get(qdenv.QodanaEnv)
							if currentQodanaEnv != tc.envExpected {
								t.Errorf("%s: Expected %s, got %s", environment.name, tc.envExpected, currentQodanaEnv)
							}
							if environment.get(qdenv.QodanaJobUrl) != tc.jobUrlExpected {
								t.Errorf(
									"%s: Expected %s, got %s",
									environment.name,
									tc.jobUrlExpected,
									environment.get(qdenv.QodanaJobUrl),
								)
							}
							if environment.get(qdenv.QodanaRemoteUrl) != tc.remoteUrlExpected {
								t.Errorf(
									"%s: Expected %s, got %s",
									environment.name,
									tc.remoteUrlExpected,
									environment.get(qdenv.QodanaRemoteUrl),
								)
							}
							if environment.get(qdenv.QodanaRevision) != tc.revisionExpected {
								t.Errorf(
									"%s: Expected %s, got %s",
									environment.name,
									revisionExpected,
									environment.get(qdenv.QodanaRevision),
								)
							}
							if environment.get(qdenv.QodanaBranch) != tc.branchExpected {
								t.Errorf(
									"%s: Expected %s, got %s",
									environment.name,
									branchExpected,
									environment.get(qdenv.QodanaBranch),
								)
							}
						},
					)
				}

				for _, k := range append(
					maps.Keys(tc.variables),
					[]string{
						qdenv.QodanaJobUrl,
						qdenv.QodanaEnv,
						qdenv.QodanaRemoteUrl,
						qdenv.QodanaRevision,
						qdenv.QodanaBranch,
					}...,
				) {
					err := os.Unsetenv(k)
					if err != nil {
						t.Fatal(err)
					}
				}
			},
		)
	}
}
