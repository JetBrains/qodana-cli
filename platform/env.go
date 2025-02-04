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
	cienvironment "github.com/cucumber/ci-environment/go"
	log "github.com/sirupsen/logrus"
	"net/url"
	"os"
	"runtime"
	"strings"
)

const (
	QodanaLicenseOnlyToken   = "QODANA_LICENSE_ONLY_TOKEN"
	QodanaToken              = "QODANA_TOKEN"
	QodanaRemoteUrl          = "QODANA_REMOTE_URL"
	QodanaDockerEnv          = "QODANA_DOCKER"
	QodanaToolEnv            = "QODANA_TOOL"
	QodanaConfEnv            = "QODANA_CONF"
	QodanaClearKeyring       = "QODANA_CLEAR_KEYRING"
	qodanaEnv                = "QODANA_ENV"
	qodanaJobUrl             = "QODANA_JOB_URL"
	QodanaBranch             = "QODANA_BRANCH"
	QodanaRevision           = "QODANA_REVISION"
	QodanaCliContainerName   = "QODANA_CLI_CONTAINER_NAME"
	QodanaCliContainerKeep   = "QODANA_CLI_CONTAINER_KEEP"
	QodanaCliUsePodman       = "QODANA_CLI_USE_PODMAN"
	QodanaDistEnv            = "QODANA_DIST"
	QodanaCorettoSdk         = "QODANA_CORETTO_SDK"
	AndroidSdkRoot           = "ANDROID_SDK_ROOT"
	QodanaLicense            = "QODANA_LICENSE"
	QodanaTreatAsRelease     = "QODANA_TREAT_AS_RELEASE"
	QodanaProjectIdHash      = "QODANA_PROJECT_ID_HASH"
	QodanaOrganisationIdHash = "QODANA_ORGANISATION_ID_HASH"
	qodanaNugetUrl           = "QODANA_NUGET_URL"
	qodanaNugetUser          = "QODANA_NUGET_USER"
	qodanaNugetPassword      = "QODANA_NUGET_PASSWORD"
	qodanaNugetName          = "QODANA_NUGET_NAME"
	gemHome                  = "GEM_HOME"
	bundleAppConfig          = "BUNDLE_APP_CONFIG"
)

// ExtractQodanaEnvironment extracts Qodana environment variables from the current environment.
func ExtractQodanaEnvironment(setEnvironmentFunc func(string, string)) {
	if license := os.Getenv(QodanaLicense); license != "" {
		setEnvironmentFunc(QodanaLicense, license)
	}
	if endpoint := os.Getenv(cloud.QodanaEndpointEnv); endpoint != "" {
		setEnvironmentFunc(cloud.QodanaEndpointEnv, endpoint)
	}
	if remoteUrl := os.Getenv(QodanaRemoteUrl); remoteUrl != "" {
		setEnvironmentFunc(QodanaRemoteUrl, remoteUrl)
	}
	if branch := os.Getenv(QodanaBranch); branch != "" {
		setEnvironmentFunc(QodanaBranch, branch)
	}
	if revision := os.Getenv(QodanaRevision); revision != "" {
		setEnvironmentFunc(QodanaRevision, revision)
	}
	ci := cienvironment.DetectCIEnvironment()
	qEnv := "cli"
	if ci != nil {
		qEnv = getCIName(ci)
		setEnvironmentFunc(qodanaJobUrl, validateJobUrl(ci.URL, qEnv))
		if ci.Git != nil {
			setEnvironmentFunc(QodanaRemoteUrl, validateRemoteUrl(ci.Git.Remote, qEnv))
			setEnvironmentFunc(QodanaBranch, validateBranch(ci.Git.Branch, qEnv))
			setEnvironmentFunc(QodanaRevision, ci.Git.Revision)
		}
		setEnvironmentFunc(qodanaNugetUrl, os.Getenv(qodanaNugetUrl))
		setEnvironmentFunc(qodanaNugetUser, os.Getenv(qodanaNugetUser))
		setEnvironmentFunc(qodanaNugetPassword, os.Getenv(qodanaNugetPassword))
		setEnvironmentFunc(qodanaNugetName, os.Getenv(qodanaNugetName))
	} else if space := os.Getenv("JB_SPACE_API_URL"); space != "" {
		qEnv = "space"
		setEnvironmentFunc(qodanaJobUrl, os.Getenv("JB_SPACE_EXECUTION_URL"))
		setEnvironmentFunc(QodanaRemoteUrl, getSpaceRemoteUrl())
		setEnvironmentFunc(QodanaBranch, os.Getenv("JB_SPACE_GIT_BRANCH"))
		setEnvironmentFunc(QodanaRevision, os.Getenv("JB_SPACE_GIT_REVISION"))
	} else if IsBitBucket() {
		qEnv = "bitbucket"
		setEnvironmentFunc(qodanaJobUrl, getBitBucketJobUrl())
	}
	setEnvironmentFunc(qodanaEnv, fmt.Sprintf("%s:%s", qEnv, Version))
}

func getCIName(ci *cienvironment.CiEnvironment) string {
	return strings.ReplaceAll(strings.ToLower(ci.Name), " ", "-")
}

func validateRemoteUrl(remote string, qEnv string) string {
	if strings.HasPrefix(qEnv, "space") {
		return getSpaceRemoteUrl()
	}
	_, err := url.ParseRequestURI(remote)
	if remote == "" || err != nil {
		log.Warnf("Unable to parse git remote URL %s, set %s env variable for proper qodana.cloud reporting", remote, QodanaRemoteUrl)
		return ""
	}
	return remote
}

func validateBranch(branch string, env string) string {
	if branch == "" {
		if env == "github-actions" {
			branch = os.Getenv("GITHUB_REF_NAME")
		} else if env == "azure-pipelines" {
			branch = os.Getenv("BUILD_SOURCEBRANCHNAME")
		} else if env == "jenkins" {
			branch = os.Getenv("GIT_BRANCH")
		}
	}
	if branch == "" {
		log.Warnf("Unable to parse git branch, set %s env variable for proper qodana.cloud reporting", QodanaBranch)
		return ""
	}
	return branch
}

func validateJobUrl(ciUrl string, qEnv string) string {
	if strings.HasPrefix(qEnv, "azure") { // temporary workaround for Azure Pipelines
		return getAzureJobUrl()
	}
	_, err := url.ParseRequestURI(ciUrl)
	if err != nil {
		return ""
	}
	return ciUrl
}

// Bootstrap takes the given command (from CLI or qodana.yaml) and runs it.
func Bootstrap(command string, project string) {
	if command != "" {
		var executor string
		var flag string
		switch runtime.GOOS {
		case "windows":
			executor = "cmd"
			flag = "/c"
		default:
			executor = "sh"
			flag = "-c"
		}

		if res, err := RunCmd(project, executor, flag, "\""+command+"\""); res > 0 || err != nil {
			log.Printf("Provided bootstrap command finished with error: %d. Exiting...", res)
			os.Exit(res)
		}
	}
}

func SetEnv(key string, value string) {
	log.Debugf("Setting %s=%s", key, value)
	if os.Getenv(key) == "" && value != "" {
		err := os.Setenv(key, value)
		if err != nil {
			return
		}
		log.Debugf("Set %s=%s", key, value)
	}
}

func UnsetRubyVariables() {
	variables := []string{gemHome, bundleAppConfig}
	for _, variable := range variables {
		if err := os.Unsetenv(variable); err != nil {
			log.Warnf("couldn't unset env variable %s", err.Error())
		}
	}
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

// getSpaceJobUrl returns the Space job URL.
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

// IsGitLab returns true if the current environment is GitLab CI.
func IsGitLab() bool {
	return os.Getenv("GITLAB_CI") == "true"
}

// IsBitBucket returns true if the current environment is BitBucket Pipelines.
func IsBitBucket() bool {
	return os.Getenv("BITBUCKET_PIPELINE_UUID") != ""
}

// isBitBucketPipe returns true if the current environment is in a working BitBucket Pipe.
func isBitBucketPipe() bool {
	return os.Getenv("BITBUCKET_PIPE_STORAGE_DIR") != "" || os.Getenv("BITBUCKET_PIPE_SHARED_STORAGE_DIR") != ""
}

func getBitBucketJobUrl() string {
	return fmt.Sprintf("https://bitbucket.org/%s/pipelines/results/%s", getBitBucketRepoFullName(), os.Getenv("BITBUCKET_BUILD_NUMBER"))
}

// getBitBucketCommit returns the BitBucket commit hash.
func getBitBucketCommit() string {
	return os.Getenv("BITBUCKET_COMMIT")
}

// getBitBucketRepoFullName returns the BitBucket repository slug in the form "owner/repo".
func getBitBucketRepoFullName() string {
	return os.Getenv("BITBUCKET_REPO_FULL_NAME")
}

func getBitBucketRepoOwner() string {
	return strings.Split(getBitBucketRepoFullName(), "/")[0]
}

func getBitBucketRepoName() string {
	return strings.Split(getBitBucketRepoFullName(), "/")[1]
}
