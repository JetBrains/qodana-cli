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

package core

import (
	"encoding/json"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/cloud"
	"github.com/JetBrains/qodana-cli/v2024/platform/git"
	"github.com/JetBrains/qodana-cli/v2024/platform/utils"
	"sort"
	"strings"
)

// various variables for parsing git log output.
var (
	gitFormatSep = "||" // separator for git log format
	gitFormat    = strings.Join(
		[]string{
			"%ae", // author mail
			"%an", // author name
			"%H",  // commit hash, in full SHA-256 format
			"%ai", // author date, ISO 8601-like format
		},
		gitFormatSep,
	)
)

const qodanaBotEmail = "qodana-support@jetbrains.com"

// author struct represents a git commit author.
type author struct {
	Email    string `json:"email"`
	Username string `json:"username"`
}

// getId() returns the author's email if it is not empty, otherwise it returns the username.
func (a *author) getId() string {
	if a.Email != "" {
		return a.Email
	}
	return a.Username
}

// isBot returns true if the author is a bot.
func (a *author) isBot() bool {
	return strings.HasSuffix(a.Email, cloud.GitHubBotSuffix) || utils.Contains(cloud.CommonGitBots, a.Email)
}

// commit struct represents a git commit.
type commit struct {
	Author *author `json:"-"`    // author of the commit
	Date   string  `json:"date"` // ISO 8601-like format
	Sha256 string  `json:"sha256"`
}

// contributor struct represents a git repo contributor: a pair of author and number of contributions.
type contributor struct {
	Author   *author  `json:"author"`
	Projects []string `json:"projects"`
	Count    int      `json:"count"`
	Commits  []commit `json:"commits"`
}

// ToJSON returns the JSON representation of the list of contributors.
func ToJSON(contributors []contributor) (string, error) {
	output := map[string]interface{}{
		"total":        len(contributors),
		"contributors": contributors,
	}
	out, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal json: %w", err)
	}
	return string(out), nil
}

// parseCommits returns the list of commits for future processing.
func parseCommits(gitLogOutput []string, excludeBots bool) []commit {
	var commits []commit
	for _, line := range gitLogOutput {
		fields := strings.Split(line, gitFormatSep)
		if len(fields) != 4 {
			continue
		}
		a := author{
			Email:    fields[0],
			Username: fields[1],
		}
		if excludeBots && a.isBot() {
			continue
		}
		if a.Email == qodanaBotEmail {
			continue
		}
		commits = append(commits, commit{
			Author: &a,
			Date:   fields[3],
			Sha256: fields[2],
		})
	}
	return commits
}

// GetContributors returns the list of contributors of the git repository.
func GetContributors(repoDirs []string, days int, excludeBots bool) []contributor {
	contributorMap := make(map[string]*contributor)
	for _, repoDir := range repoDirs {
		gLog := git.GitLog(repoDir, gitFormat, days)
		for _, c := range parseCommits(gLog, excludeBots) {
			authorId := c.Author.getId()
			if i, ok := contributorMap[authorId]; ok {
				i.Count++
				i.Projects = utils.Append(i.Projects, repoDir)
				i.Commits = append(i.Commits, c)
			} else {
				contributorMap[authorId] = &contributor{
					Author:   c.Author,
					Count:    1,
					Projects: []string{repoDir},
					Commits:  []commit{c},
				}
			}
		}
	}

	contributors := make([]contributor, 0, len(contributorMap))
	for _, c := range contributorMap {
		contributors = append(contributors, *c)
	}

	sort.Slice(contributors, func(i, j int) bool {
		return contributors[i].Count > contributors[j].Count
	})

	return contributors
}
