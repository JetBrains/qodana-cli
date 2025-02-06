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

package qdenv

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform/version"
	cienvironment "github.com/cucumber/ci-environment/go"
	log "github.com/sirupsen/logrus"
	"net/url"
	"os"
	"strings"
)

const (
	QodanaLicenseOnlyToken        = "QODANA_LICENSE_ONLY_TOKEN"
	QodanaToken                   = "QODANA_TOKEN"
	QodanaRemoteUrl               = "QODANA_REMOTE_URL"
	QodanaDockerEnv               = "QODANA_DOCKER"
	QodanaToolEnv                 = "QODANA_TOOL"
	QodanaConfEnv                 = "QODANA_CONF"
	QodanaClearKeyring            = "QODANA_CLEAR_KEYRING"
	QodanaEnv                     = "QODANA_ENV"
	QodanaJobUrl                  = "QODANA_JOB_URL"
	QodanaBranch                  = "QODANA_BRANCH"
	QodanaRevision                = "QODANA_REVISION"
	QodanaCliContainerName        = "QODANA_CLI_CONTAINER_NAME"
	QodanaCliContainerKeep        = "QODANA_CLI_CONTAINER_KEEP"
	QodanaCliUsePodman            = "QODANA_CLI_USE_PODMAN"
	QodanaDistEnv                 = "QODANA_DIST"
	QodanaCorettoSdk              = "QODANA_CORETTO_SDK"
	AndroidSdkRoot                = "ANDROID_SDK_ROOT"
	QodanaLicense                 = "QODANA_LICENSE"
	QodanaTreatAsRelease          = "QODANA_TREAT_AS_RELEASE"
	QodanaProjectIdHash           = "QODANA_PROJECT_ID_HASH"
	QodanaOrganisationIdHash      = "QODANA_ORGANISATION_ID_HASH"
	QodanaNugetUrl                = "QODANA_NUGET_URL"
	QodanaNugetUser               = "QODANA_NUGET_USER"
	QodanaNugetPassword           = "QODANA_NUGET_PASSWORD"
	QodanaNugetName               = "QODANA_NUGET_NAME"
	GemHome                       = "GEM_HOME"
	BundleAppConfig               = "BUNDLE_APP_CONFIG"
	QodanaEndpointEnv             = "QODANA_ENDPOINT"
	QodanaCloudRequestCooldownEnv = "QODANA_CLOUD_REQUEST_COOLDOWN"
	QodanaCloudRequestTimeoutEnv  = "QODANA_CLOUD_REQUEST_TIMEOUT"
	QodanaCloudRequestRetriesEnv  = "QODANA_CLOUD_REQUEST_RETRIES"
)

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

func IsContainer() bool {
	return os.Getenv(QodanaDockerEnv) != ""
}

// ExtractQodanaEnvironment extracts Qodana environment variables from the current environment.
func ExtractQodanaEnvironment(setEnvironmentFunc func(string, string)) {
	if license := os.Getenv(QodanaLicense); license != "" {
		setEnvironmentFunc(QodanaLicense, license)
	}
	if endpoint := os.Getenv(QodanaEndpointEnv); endpoint != "" {
		setEnvironmentFunc(QodanaEndpointEnv, endpoint)
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
		qEnv = GetCIName(ci)
		setEnvironmentFunc(QodanaJobUrl, validateJobUrl(ci.URL, qEnv))
		if ci.Git != nil {
			setEnvironmentFunc(QodanaRemoteUrl, validateRemoteUrl(ci.Git.Remote, qEnv))
			setEnvironmentFunc(QodanaBranch, validateBranch(ci.Git.Branch, qEnv))
			setEnvironmentFunc(QodanaRevision, ci.Git.Revision)
		}
		setEnvironmentFunc(QodanaNugetUrl, os.Getenv(QodanaNugetUrl))
		setEnvironmentFunc(QodanaNugetUser, os.Getenv(QodanaNugetUser))
		setEnvironmentFunc(QodanaNugetPassword, os.Getenv(QodanaNugetPassword))
		setEnvironmentFunc(QodanaNugetName, os.Getenv(QodanaNugetName))
	} else if space := os.Getenv("JB_SPACE_API_URL"); space != "" {
		qEnv = "space"
		setEnvironmentFunc(QodanaJobUrl, os.Getenv("JB_SPACE_EXECUTION_URL"))
		setEnvironmentFunc(QodanaRemoteUrl, getSpaceRemoteUrl())
		setEnvironmentFunc(QodanaBranch, os.Getenv("JB_SPACE_GIT_BRANCH"))
		setEnvironmentFunc(QodanaRevision, os.Getenv("JB_SPACE_GIT_REVISION"))
	} else if IsBitBucket() {
		qEnv = "bitbucket"
		setEnvironmentFunc(QodanaJobUrl, GetBitBucketJobUrl())
	}
	setEnvironmentFunc(QodanaEnv, fmt.Sprintf("%s:%s", qEnv, version.Version))
}

func GetCIName(ci *cienvironment.CiEnvironment) string {
	return strings.ReplaceAll(strings.ToLower(ci.Name), " ", "-")
}

func validateRemoteUrl(remote string, qEnv string) string {
	if strings.HasPrefix(qEnv, "space") {
		return getSpaceRemoteUrl()
	}
	_, err := url.ParseRequestURI(remote)
	if remote == "" || err != nil {
		log.Warnf(
			"Unable to parse git remote URL %s, set %s env variable for proper qodana.cloud reporting",
			remote,
			QodanaRemoteUrl,
		)
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

func UnsetRubyVariables() {
	variables := []string{GemHome, BundleAppConfig}
	for _, variable := range variables {
		if err := os.Unsetenv(variable); err != nil {
			log.Warnf("couldn't unset env variable %s", err.Error())
		}
	}
}

// getAzureJobUrl returns the Azure Pipelines job URL.
func getAzureJobUrl() string {
	if server := os.Getenv("SYSTEM_TEAMFOUNDATIONCOLLECTIONURI"); server != "" {
		return strings.Join(
			[]string{
				server,
				os.Getenv("SYSTEM_TEAMPROJECT"),
				"/_build/results?buildId=",
				os.Getenv("BUILD_BUILDID"),
			}, "",
		)
	}
	return ""
}

// getSpaceJobUrl returns the Space job URL.
func getSpaceRemoteUrl() string {
	if server := os.Getenv("JB_SPACE_API_URL"); server != "" {
		return strings.Join(
			[]string{
				"ssh://git@git.",
				server,
				"/",
				os.Getenv("JB_SPACE_PROJECT_KEY"),
				"/",
				os.Getenv("JB_SPACE_GIT_REPOSITORY_NAME"),
				".git",
			}, "",
		)
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

// IsBitBucketPipe returns true if the current environment is in a working BitBucket Pipe.
func IsBitBucketPipe() bool {
	return os.Getenv("BITBUCKET_PIPE_STORAGE_DIR") != "" || os.Getenv("BITBUCKET_PIPE_SHARED_STORAGE_DIR") != ""
}

func GetBitBucketJobUrl() string {
	return fmt.Sprintf(
		"https://bitbucket.org/%s/pipelines/results/%s",
		GetBitBucketRepoFullName(),
		os.Getenv("BITBUCKET_BUILD_NUMBER"),
	)
}

// GetBitBucketCommit returns the BitBucket commit hash.
func GetBitBucketCommit() string {
	return os.Getenv("BITBUCKET_COMMIT")
}

// GetBitBucketRepoFullName returns the BitBucket repository slug in the form "owner/repo".
func GetBitBucketRepoFullName() string {
	return os.Getenv("BITBUCKET_REPO_FULL_NAME")
}

func GetBitBucketRepoOwner() string {
	return strings.Split(GetBitBucketRepoFullName(), "/")[0]
}

func GetBitBucketRepoName() string {
	return strings.Split(GetBitBucketRepoFullName(), "/")[1]
}
