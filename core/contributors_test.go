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

import "testing"

func TestGetContributors(t *testing.T) {
	contributors := GetContributors([]string{"."}, -1, false)
	if len(contributors) == 0 {
		t.Error("Expected at least one contributor or you need to update the test repo")
	}

	numBotContributors := countContributors(func(c contributor) bool {
		return c.Author.Username == "dependabot[bot]"
	}, contributors)
	if numBotContributors < 1 {
		t.Error("Expected dependabot[bot] contributor")
	}

	numContributorsWithSameEmail := countContributors(func(c contributor) bool {
		return c.Author.Email == "dmitry.golovinov@jetbrains.com"
	}, contributors)
	if numContributorsWithSameEmail < 2 {
		t.Error("Expected contributor with same email but different username to be counted multiple times")
	}
}

func TestParseCommits(t *testing.T) {
	gitLogOutput := []string{
		"me@me.com||me||0e64c1b093d07762ffd28c0faec75a55f67c2260||2023-05-05 16:11:38 +0200",
		"me@me.com||me||0e64c1b093d07762ffd28c0faec75a55f67c2260||2023-05-05 16:11:38 +0200",
	}

	commits := parseCommits(gitLogOutput, true)

	expectedCount := 2
	if len(commits) != expectedCount {
		t.Fatalf("Expected %d commits, got %d", expectedCount, len(commits))
	}

	expectedSha256 := "0e64c1b093d07762ffd28c0faec75a55f67c2260"
	if commits[0].Sha256 != expectedSha256 {
		t.Errorf("Expected SHA256 %s, got %s", expectedSha256, commits[0].Sha256)
	}

	expectedDate := "2023-05-05 16:11:38 +0200"
	if commits[1].Date != expectedDate {
		t.Errorf("Expected date %s, got %s", expectedDate, commits[1].Date)
	}
}

func countContributors(matches func(contributor) bool, contributors []contributor) int {
	result := 0
	for _, c := range contributors {
		if matches(c) {
			result += 1
		}
	}
	return result
}
