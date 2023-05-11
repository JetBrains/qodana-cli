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

// author struct represents a git commit author.
type author struct {
	Email    string
	Username string
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
	return strings.HasSuffix(a.Email, gitHubBotSuffix) || contains(commonGitBots, a.Email)
}

// commit struct represents a git commit.
type commit struct {
	Author *author
	Date   string
	Sha256 string
}

// contributor struct represents a git repo contributor: pair of author and number of contributions.
type contributor struct {
	Author        *author
	Contributions int
}

// getCommits returns the list of commits for future processing.
func getCommits(repoDir string, days int, excludeBots bool) []commit {
	var commits []commit
	for _, line := range gitLog(repoDir, gitFormat, days, true) {
		fields := strings.Split(line, gitFormatSep)
		if len(fields) != 4 {
			continue
		}
		author := author{
			Email:    fields[0],
			Username: fields[1],
		}
		if excludeBots && author.isBot() {
			continue
		}
		commits = append(commits, commit{
			Author: &author,
			Date:   fields[2],
			Sha256: fields[3],
		})
	}
	return commits
}

// GetContributors returns the list of contributors of the git repository.
func GetContributors(repoDir string, days int, excludeBots bool) []contributor {
	contributorMap := make(map[string]*contributor)
	for _, commit := range getCommits(repoDir, days, excludeBots) {
		authorId := commit.Author.getId()
		if c, ok := contributorMap[authorId]; ok {
			c.Contributions++
		} else {
			contributorMap[authorId] = &contributor{
				Author:        commit.Author,
				Contributions: 1,
			}
		}
	}

	contributors := make([]contributor, 0, len(contributorMap))
	for _, contributor := range contributorMap {
		contributors = append(contributors, *contributor)
	}

	sort.Slice(contributors, func(i, j int) bool {
		return contributors[i].Contributions > contributors[j].Contributions
	})

	return contributors
}
