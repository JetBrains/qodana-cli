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
	"github.com/JetBrains/qodana-cli/v2024/platform/utils"
	"github.com/JetBrains/qodana-cli/v2024/sarif"
	"os"
	"strings"
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
	if os.Getenv("QODANA_BRANCH") != "" {
		ret.Branch = os.Getenv("QODANA_BRANCH")
	} else {
		branch, err := getBranchName(pwd)
		if err != nil {
			return ret, err
		}
		ret.Branch = branch
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
	uri, _, ret, err := utils.RunCmdRedirectOutput(pwd, "git", "ls-remote", "--get-url")
	if err != nil {
		return "", err
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
	branch, _, ret, err := utils.RunCmdRedirectOutput(pwd, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	if ret != 0 {
		return "", errors.New("git rev-parse --abbrev-ref HEAD failed")
	}
	return strings.TrimSpace(branch), nil
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
