/*
 * Copyright 2021-2022 JetBrains s.r.o.
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
	"os"
	"strings"
)

/*
 * Azure – https://docs.microsoft.com/en-us/azure/devops/pipelines/build/variables?view=azure-devops&tabs=yaml
 * BitBucket – https://support.atlassian.com/bitbucket-cloud/docs/variables-and-secrets/
 * BuildKite – https://buildkite.com/docs/pipelines/environment-variables
 * CircleCI – https://circleci.com/docs/2.0/env-vars#built-in-environment-variables
 * GitHub – https://docs.github.com/en/actions/learn-github-actions/environment-variables#default-environment-variables
 * GitLab – https://docs.gitlab.com/ee/ci/variables/predefined_variables.html
 * Jenkins –
https://www.perforce.com/manuals/jenkins/Content/P4Jenkins/variable-expansion.html#Built_in_environment_variables
 * Space – https://www.jetbrains.com/help/space/automation-environment-variables.html#general

 * This list will be extended in the future.
Right now it's not possible to properly detect Azure server environment.
 * Some services do not provide the repo URL (BuildKite, Jenkins), some do not have job URL at the moment (Space).
*/

// getQodanaEnv returns the environment name.
func getQodanaEnv() string {
	env := "cli"
	if userDefined := os.Getenv(qodanaEnv); userDefined != "" {
		return userDefined
	} else if bitbucket := os.Getenv("BITBUCKET_GIT_HTTP_ORIGIN"); bitbucket != "" {
		return "bitbucket"
	} else if buildkite := os.Getenv("BUILDKITE_BUILD_URL"); buildkite != "" {
		return "buildkite"
	} else if circleci := os.Getenv("CIRCLE_BUILD_URL"); circleci != "" {
		return "circleci"
	} else if jenkins := os.Getenv("BUILD_URL"); jenkins != "" {
		return "jenkins"
	} else if gitlabci := os.Getenv("GITLAB_CI"); gitlabci != "" {
		return "gitlabci"
	} else if github := os.Getenv("GITHUB_SERVER_URL"); github != "" {
		if strings.HasPrefix(github, "https://github.com") {
			return "github"
		}
		return "github-enterprise"
	} else if space := os.Getenv("JB_SPACE_API_URL"); space != "" {
		if !strings.Contains(space, "jetbrains.space") {
			return "space-onpremise"
		}
		return "space"
	}
	return env
}

// getQodanaJobUrl returns the job URL.
func getQodanaJobUrl() string {
	url := ""
	if url = os.Getenv(qodanaJobUrl); url != "" { // User defined
		return url
	} else if url = getAzureJobUrl(); url != "" { // Azure
		return url
	} else if url = getBitBucketJobUrl(); url != "" { // BitBucket
		return url
	} else if url = os.Getenv("BUILDKITE_BUILD_URL"); url != "" { // BuildKite
		return url
	} else if url = os.Getenv("CIRCLE_BUILD_URL"); url != "" { // CircleCI
		return url
	} else if url = os.Getenv("CI_JOB_URL"); url != "" { // GitLab CI
		return url
	} else if url = getGitHubJobUrl(); url != "" { // GitHub Actions
		return url
	} else if url = os.Getenv("BUILD_URL"); url != "" { // Jenkins
		return url
	}
	return url
}

// getQodanaRepoUrl returns the repository URL.
func getQodanaRepoUrl() string {
	url := ""
	if url = os.Getenv(qodanaRepoUrl); url != "" { // User defined
		return url
	} else if url = os.Getenv("BUILD_REPOSITORY_URI"); url != "" { // Azure
		return url
	} else if url = os.Getenv("BITBUCKET_GIT_HTTP_ORIGIN"); url != "" { // BitBucket
		return url
	} else if url = getGitHubRepoUrl(); url != "" { // GitHub
		return url
	} else if url = os.Getenv("CI_REPOSITORY_URL"); url != "" { // GitLab CI
		return url
	} else if url = getSpaceRepoUrl(); url != "" { // Space
		return url
	}
	return url
}

// getQodanaRemoteUrl returns the repository URL.
func getQodanaRemoteUrl() string {
	url := ""
	if url = os.Getenv(qodanaRemoteUrl); url != "" { // User defined
		return url
	} else if url = os.Getenv("BUILD_REPOSITORY_URI"); url != "" { // Azure
		return url
	} else if url = os.Getenv("BITBUCKET_GIT_SSH_ORIGIN"); url != "" { // BitBucket
		return url
	} else if url = getGitHubRemoteUrl(); url != "" { // GitHub
		return url
	} else if url = os.Getenv("CI_REPOSITORY_URL"); url != "" { // GitLab CI
		return url
	} else if url = getSpaceRemoteUrl(); url != "" { // Space
		return url
	}
	return url
}

// getQodanaBranch returns the branch name.
func getQodanaBranch() string {
	branch := ""
	if branch = os.Getenv(qodanaBranch); branch != "" { // User defined
		return branch
	} else if branch = os.Getenv("BUILD_SOURCEBRANCHNAME"); branch != "" { // Azure
		return branch
	} else if branch = os.Getenv("BITBUCKET_BRANCH"); branch != "" { // BitBucket
		return branch
	} else if branch = os.Getenv("BUILDKITE_BRANCH"); branch != "" { // BuildKite
		return branch
	} else if branch = os.Getenv("CIRCLE_BRANCH"); branch != "" { // CircleCI
		return branch
	} else if branch = os.Getenv("CI_COMMIT_REF_NAME"); branch != "" { // GitLab CI
		return branch
	} else if branch = getGitHubBranch(); branch != "" { // GitHub Actions
		return branch
	} else if branch = os.Getenv("BRANCH_NAME"); branch != "" { // Jenkins
		return branch
	} else if branch = os.Getenv("JB_SPACE_GIT_BRANCH"); branch != "" { // Space
		return branch
	}
	return branch
}

// getQodanaRevision returns the commit hash.
func getQodanaRevision() string {
	revision := ""
	if revision = os.Getenv(qodanaRevision); revision != "" { // User defined
		return revision
	} else if revision = os.Getenv("BUILD_SOURCEVERSION"); revision != "" { // Azure
		return revision
	} else if revision = os.Getenv("BITBUCKET_COMMIT"); revision != "" { // BitBucket
		return revision
	} else if revision = os.Getenv("BUILDKITE_COMMIT"); revision != "" { // BuildKite
		return revision
	} else if revision = os.Getenv("CIRCLE_SHA1"); revision != "" { // CircleCI
		return revision
	} else if revision = os.Getenv("CI_COMMIT_SHA"); revision != "" { // GitLab CI
		return revision
	} else if revision = os.Getenv("GITHUB_SHA"); revision != "" { // GitHub Actions
		return revision
	} else if revision = os.Getenv("GIT_COMMIT"); revision != "" { // Jenkins
		return revision
	} else if revision = os.Getenv("JB_SPACE_GIT_REVISION"); revision != "" { // Space
		return revision
	}
	return revision
}

// getAzureJobUrl returns the Azure Pipelines job URL.
func getAzureJobUrl() string {
	if server := os.Getenv("SYSTEM_TEAMFOUNDATIONCOLLECTIONURI"); server != "" {
		return strings.Join([]string{
			server,
			os.Getenv("SYSTEM_TEAMPROJECT"),
			"/_build/results?buildId=",
			os.Getenv("BUILD_BUILDID"),
		}, "")
	}
	return ""
}

// getBitBucketJobUrl returns the BitBucket job URL.
func getBitBucketJobUrl() string {
	if repo := getBitBucketRepoUrl(); repo != "" {
		return strings.Join([]string{
			repo,
			"/addon/pipelines/home#!/results/",
			os.Getenv("BITBUCKET_PIPELINE_UUID"),
		}, "")
	}
	return ""
}

// getGitHubJobUrl returns the GitHub Actions job URL.
func getGitHubJobUrl() string {
	if repo := getGitHubRepoUrl(); repo != "" {
		return strings.Join([]string{
			repo,
			"/actions/runs/",
			os.Getenv("GITHUB_RUN_ID"),
		}, "")
	}
	return ""
}

// getSpaceRepoUrl returns the Space repository URL.
func getSpaceRepoUrl() string {
	if server := os.Getenv("JB_SPACE_API_URL"); server != "" {
		return strings.Join([]string{
			server,
			"/p/",
			os.Getenv("JB_SPACE_PROJECT_KEY"),
			"/repositories/",
			os.Getenv("JB_SPACE_GIT_REPOSITORY_NAME"),
		}, "")
	}
	return ""
}

// getBitBucketRepoUrl returns the BitBucket repository URL.
func getBitBucketRepoUrl() string {
	if repo := os.Getenv("BITBUCKET_GIT_HTTP_ORIGIN"); repo != "" {
		return repo
	}
	return ""
}

// getGitHubRepoUrl returns the GitHub repository URL.
func getGitHubRepoUrl() string {
	if repo := os.Getenv("GITHUB_REPOSITORY"); repo != "" {
		return os.Getenv("GITHUB_SERVER_URL") + "/" + os.Getenv("GITHUB_REPOSITORY")
	}
	return ""
}

// getGitHubRemoteUrl returns the GitHub repository remote URL.
func getGitHubRemoteUrl() string {
	if repo := os.Getenv("GITHUB_REPOSITORY"); repo != "" {
		return strings.Join([]string{
			"git@",
			strings.TrimPrefix(os.Getenv("GITHUB_SERVER_URL"), "https://"),
			":",
			os.Getenv("GITHUB_REPOSITORY"),
			".git",
		}, "")
	}
	return ""
}

// getSpaceRemoteUrl returns the Space repository URL.
func getSpaceRemoteUrl() string {
	if server := os.Getenv("JB_SPACE_API_URL"); server != "" {
		return strings.Join([]string{
			"ssh://git@git.",
			server,
			"/",
			os.Getenv("JB_SPACE_PROJECT_KEY"),
			"/",
			os.Getenv("JB_SPACE_GIT_REPOSITORY_NAME"),
			".git",
		}, "")
	}
	return ""
}

// getGitHubBranch returns the GitHub branch name.
func getGitHubBranch() string {
	if branch := os.Getenv("GITHUB_HEAD_REF"); branch != "" {
		return "refs/heads/" + branch
	} else if branch = os.Getenv("GITHUB_REF"); branch != "" {
		return branch
	}
	return ""
}
