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
	"sort"
	"strings"
)

// various variables for parsing git log output.
var (
	gitFormatSep = "||" // separator for git log format
	gitFormat    = strings.Join(
		[]string{
			"%aE", // author mail, respecting .mailmap
			"%aN", // author name, respecting .mailmap
			"%H",  // commit hash, in full SHA-256 format
			"%ai", // author date, ISO 8601-like format
		},
		gitFormatSep,
	)
)

// Author struct represents a git commit author.
type Author struct {
	Email    string
	Username string
}

// getId() returns the author's email if it is not empty, otherwise it returns the username.
func (a *Author) getId() string {
	if a.Email != "" {
		return a.Email
	}
	return a.Username
}

// isBot returns true if the author is a bot.
func (a *Author) isBot() bool {
	return strings.HasSuffix(a.Email, gitHubBotSuffix) || contains(commonGitBots, a.Email)
}

// Commit struct represents a git commit.
type Commit struct {
	Author *Author
	Date   string
	Sha256 string
}

// Contributor struct represents a git repo contributor: pair of Author and number of contributions.
type Contributor struct {
	Author        *Author
	Contributions int
}

// getCommits returns the list of commits for future processing.
func getCommits(repoDir string, days int, excludeBots bool) []Commit {
	var commits []Commit
	for _, line := range gitLog(repoDir, gitFormat, days, true) {
		fields := strings.Split(line, gitFormatSep)
		if len(fields) != 4 {
			continue
		}
		author := Author{
			Email:    fields[0],
			Username: fields[1],
		}
		if excludeBots && author.isBot() {
			continue
		}
		commits = append(commits, Commit{
			Author: &author,
			Date:   fields[2],
			Sha256: fields[3],
		})
	}
	return commits
}

// GetContributors returns the list of contributors of the git repository.
func GetContributors(repoDir string, days int, excludeBots bool) []Contributor {
	contributorMap := make(map[string]*Contributor)
	for _, commit := range getCommits(repoDir, days, excludeBots) {
		authorId := commit.Author.getId()
		if contributor, ok := contributorMap[authorId]; ok {
			contributor.Contributions++
		} else {
			contributorMap[authorId] = &Contributor{
				Author:        commit.Author,
				Contributions: 1,
			}
		}
	}

	contributors := make([]Contributor, 0, len(contributorMap))
	for _, contributor := range contributorMap {
		contributors = append(contributors, *contributor)
	}

	sort.Slice(contributors, func(i, j int) bool {
		return contributors[i].Contributions > contributors[j].Contributions
	})

	return contributors
}
