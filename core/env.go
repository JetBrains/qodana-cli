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
	"github.com/JetBrains/qodana-cli/v2023/platform"
	cienvironment "github.com/cucumber/ci-environment/go"
	log "github.com/sirupsen/logrus"
	"net/url"
	"os"
	"runtime"
	"strings"
)

const ( // TODO : move to platform
	qodanaEnv              = "QODANA_ENV"
	qodanaJobUrl           = "QODANA_JOB_URL"
	qodanaBranch           = "QODANA_BRANCH"
	qodanaRevision         = "QODANA_REVISION"
	qodanaCliContainerName = "QODANA_CLI_CONTAINER_NAME"
	qodanaCliContainerKeep = "QODANA_CLI_CONTAINER_KEEP"
	qodanaCliUsePodman     = "QODANA_CLI_USE_PODMAN"
	QodanaDistEnv          = "QODANA_DIST"
	qodanaCorettoSdk       = "QODANA_CORETTO_SDK"
	androidSdkRoot         = "ANDROID_SDK_ROOT"
	QodanaLicense          = "QODANA_LICENSE"
	QodanaProjectIdHash    = "QODANA_PROJECT_ID_HASH"
	QodanaTreatAsRelease   = "QODANA_TREAT_AS_RELEASE"
	qodanaNugetUrl         = "QODANA_NUGET_URL"
	qodanaNugetUser        = "QODANA_NUGET_USER"
	qodanaNugetPassword    = "QODANA_NUGET_PASSWORD"
	qodanaNugetName        = "QODANA_NUGET_NAME"
	qodanaRepoUrl          = "QODANA_REPO_URL"
)

// ExtractQodanaEnvironment extracts Qodana environment variables from the current environment.
func ExtractQodanaEnvironment(setEnvironmentFunc func(string, string)) {
	ci := cienvironment.DetectCIEnvironment()
	qEnv := "cli"
	if ci != nil {
		qEnv = strings.ReplaceAll(strings.ToLower(ci.Name), " ", "-")
		setEnvironmentFunc(qodanaJobUrl, validateJobUrl(ci.URL, qEnv))
		if ci.Git != nil {
			setEnvironmentFunc(platform.QodanaRemoteUrl, validateRemoteUrl(ci.Git.Remote, qEnv))
			setEnvironmentFunc(qodanaBranch, validateBranch(ci.Git.Branch, qEnv))
			setEnvironmentFunc(qodanaRevision, ci.Git.Revision)
			setEnvironmentFunc(qodanaRepoUrl, getRepositoryHttpUrl(qEnv, ci.Git.Remote))
		}
		setEnvironmentFunc(qodanaNugetUrl, os.Getenv(qodanaNugetUrl))
		setEnvironmentFunc(qodanaNugetUser, os.Getenv(qodanaNugetUser))
		setEnvironmentFunc(qodanaNugetPassword, os.Getenv(qodanaNugetPassword))
		setEnvironmentFunc(qodanaNugetName, os.Getenv(qodanaNugetName))
	} else if space := os.Getenv("JB_SPACE_API_URL"); space != "" {
		qEnv = "space"
		setEnvironmentFunc(qodanaJobUrl, os.Getenv("JB_SPACE_EXECUTION_URL"))
		setEnvironmentFunc(platform.QodanaRemoteUrl, getSpaceRemoteUrl())
		setEnvironmentFunc(qodanaBranch, os.Getenv("JB_SPACE_GIT_BRANCH"))
		setEnvironmentFunc(qodanaRevision, os.Getenv("JB_SPACE_GIT_REVISION"))
		setEnvironmentFunc(qodanaRepoUrl, getRepositoryHttpUrl(qEnv, ""))
	}
	setEnvironmentFunc(qodanaEnv, fmt.Sprintf("%s:%s", qEnv, Version))
}

func validateRemoteUrl(remote string, qEnv string) string {
	if strings.HasPrefix(qEnv, "space") {
		return getSpaceRemoteUrl()
	}
	_, err := url.ParseRequestURI(remote)
	if remote == "" || err != nil {
		log.Warnf("Unable to parse git remote URL %s, set %s env variable for proper qodana.cloud reporting", remote, platform.QodanaRemoteUrl)
		return ""
	}
	return remote
}

func getRepositoryHttpUrl(qEnv string, remoteUrl string) string {
	switch qEnv {
	case "gitlab":
		return os.Getenv("CI_PROJECT_URL") // gitlab exposes this directly, don't need to mess with the url
	case "space":
		return strings.Join([]string{
			"https://",
			os.Getenv("JB_SPACE_API_URL"),
			"/p/",
			os.Getenv("JB_SPACE_PROJECT_KEY"),
			"/repositories/",
			os.Getenv("JB_SPACE_GIT_REPOSITORY_NAME"),
		}, "")
	default:
		remoteUrl = strings.TrimSuffix(remoteUrl, ".git")
	}
	parsed, err := url.ParseRequestURI(remoteUrl)

	if remoteUrl == "" || err != nil || !strings.HasPrefix(parsed.Scheme, "http") {
		log.Warnf("Unable to parse http(s) remote URL from %s, set %s env variable for proper qodana.cloud reporting", remoteUrl, qodanaRepoUrl)
		return ""
	}

	parsed.User = nil
	return parsed.String()
}

func validateBranch(branch string, env string) string {
	if branch == "" {
		if env == "github-actions" {
			branch = os.Getenv("GITHUB_REF")
			if strings.Contains(branch, "/pull/") {
				branch = os.Getenv("GITHUB_HEAD_REF")
			}
		} else if env == "azure-pipelines" {
			branch = os.Getenv("BUILD_SOURCEBRANCHNAME")
		} else if env == "jenkins" {
			branch = os.Getenv("GIT_BRANCH")
		}
	}
	if branch == "" {
		log.Warnf("Unable to parse git branch, set %s env variable for proper qodana.cloud reporting", qodanaBranch)
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

// bootstrap takes the given command (from CLI or qodana.yaml) and runs it.
func bootstrap(command string, project string) {
	if command != "" {
		log.Printf("Running %s...", command)
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

		if res, err := platform.RunCmd(project, executor, flag, command); res > 0 || err != nil {
			log.Printf("Provided bootstrap command finished with error: %d. Exiting...", res)
			os.Exit(res)
		}
	}
}

func setEnv(key string, value string) {
	log.Debugf("Setting %s=%s", key, value)
	if os.Getenv(key) == "" && value != "" {
		err := os.Setenv(key, value)
		if err != nil {
			return
		}
		log.Debugf("Set %s=%s", key, value)
	}
}
