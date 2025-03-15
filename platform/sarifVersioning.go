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
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	cienvironment "github.com/cucumber/ci-environment/go"
	log "github.com/sirupsen/logrus"

	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
	"github.com/JetBrains/qodana-cli/v2025/sarif"
)

// it is needed for third party linters

// GetVersionDetails returns the version control details for the current repository.
func GetVersionDetails(pwd string) (sarif.VersionControlDetails, error) {
	ret := sarif.VersionControlDetails{}
	if os.Getenv("QODANA_REMOTE_URL") != "" {
		ret.RepositoryUri = os.Getenv("QODANA_REMOTE_URL") // TODO : reuse consts
	} else {
		uri, err := getRepositoryUri(pwd)
		if err != nil {
			return ret, err
		}
		ret.RepositoryUri = uri
	}

	ret.Branch = os.Getenv("QODANA_BRANCH")
	if ret.Branch == "" {
		branch, err := getBranchName(pwd)
		if err != nil {
			return ret, err
		}
		ret.Branch = branch
	}
	// Sometimes in CI the HEAD is detached even on push-based runs.
	// As a last resort, try to pick up the branch name from pre-defined environment variables.
	if ret.Branch == "" {
		ci := cienvironment.DetectCIEnvironment()
		if ci != nil && ci.Git != nil {
			ret.Branch = ci.Git.Branch
		}
	}

	if os.Getenv("QODANA_REVISION") != "" {
		ret.RevisionId = os.Getenv("QODANA_REVISION")
	} else {
		rev, err := getRevisionId(pwd)
		if err != nil {
			return ret, err
		}
		ret.RevisionId = rev
	}

	ret.Properties = &sarif.PropertyBag{}
	ret.Properties.AdditionalProperties = map[string]interface{}{
		"repoUrl":         ret.RepositoryUri,
		"vcsType":         "Git",
		"lastAuthorName":  getLastAuthorName(pwd),
		"lastAuthorEmail": getAuthorEmail(pwd),
	}
	return ret, nil
}

func getRepositoryUri(pwd string) (string, error) {
	uri, stderr, ret, err := utils.RunCmdRedirectOutput(pwd, "git", "ls-remote", "--get-url")
	if err != nil {
		return "", err
	}
	if ret == 128 {
		// Returned when a remote is not configured or multiple remotes exist, none of which is the default
		log.Warn("Failed to retrieve remote URI: ", stderr)
		uriStruct := url.URL{
			Scheme: "file",
			Host:   "",
			Path:   filepath.ToSlash(pwd),
		}
		return uriStruct.String(), nil
	}
	if ret != 0 {
		return "", errors.New("git ls-remote --get-url failed")
	}
	trimUrl := strings.TrimSpace(uri)
	if !strings.Contains(trimUrl, "://") {
		trimUrl = "ssh://" + trimUrl
	}
	return trimUrl, nil
}

func getRevisionId(pwd string) (string, error) {
	rev, _, ret, err := utils.RunCmdRedirectOutput(pwd, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	if ret != 0 {
		return "", errors.New("git rev-parse HEAD failed")
	}
	return strings.TrimSpace(rev), nil
}

func getBranchName(pwd string) (string, error) {
	// note: git branch --show-current not used because the flag is too recent at the time of writing
	branch, _, ret, err := utils.RunCmdRedirectOutput(pwd, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	if ret == 128 {
		// this approach covers some corner cases, notably when no commits exist
		branch, _, ret, err = utils.RunCmdRedirectOutput(pwd, "git", "symbolic-ref", "--short", "HEAD")
		if err != nil {
			return "", err
		}
	}
	if ret != 0 {
		return "", errors.New("git rev-parse --abbrev-ref HEAD failed")
	}
	branch = strings.TrimSpace(branch)
	if branch == "HEAD" {
		// HEAD is a reserved name in git, so HEAD can only mean that HEAD is detached.
		return "", nil
	}
	return branch, nil
}

func getLastAuthorName(pwd string) string {
	name, _, ret, err := utils.RunCmdRedirectOutput(pwd, "git", "log", "-1", "--pretty=format:%an")
	if err != nil || ret != 0 {
		return ""
	}
	return strings.TrimSpace(name)
}

func getAuthorEmail(pwd string) string {
	email, _, ret, err := utils.RunCmdRedirectOutput(pwd, "git", "log", "-1", "--pretty=format:%ae")
	if err != nil || ret != 0 {
		return ""
	}
	return strings.TrimSpace(email)
}
