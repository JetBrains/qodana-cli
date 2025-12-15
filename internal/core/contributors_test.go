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
	if numContributorsWithSameEmail != 1 {
		t.Error("Expected contributor with same email but different username to be counted once")
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

func TestAuthorGetId(t *testing.T) {
	tests := []struct {
		name     string
		author   author
		expected string
	}{
		{
			name:     "email present",
			author:   author{Email: "test@example.com", Username: "testuser"},
			expected: "test@example.com",
		},
		{
			name:     "email empty",
			author:   author{Email: "", Username: "testuser"},
			expected: "testuser",
		},
		{
			name:     "both empty",
			author:   author{Email: "", Username: ""},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.author.getId(); got != tt.expected {
				t.Errorf("getId() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAuthorIsBot(t *testing.T) {
	tests := []struct {
		name     string
		author   author
		expected bool
	}{
		{
			name:     "github bot",
			author:   author{Email: "something[bot]@users.noreply.github.com"},
			expected: true,
		},
		{
			name:     "dependabot",
			author:   author{Email: "dependabot[bot]@users.noreply.github.com"},
			expected: true,
		},
		{
			name:     "regular user",
			author:   author{Email: "user@example.com"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.author.isBot(); got != tt.expected {
				t.Errorf("isBot() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestToJSON(t *testing.T) {
	contributors := []contributor{
		{
			Author:   &author{Email: "test@example.com", Username: "test"},
			Count:    5,
			Projects: []string{"/project1"},
			Commits:  []commit{{Sha256: "abc123", Date: "2023-01-01"}},
		},
	}

	result, err := ToJSON(contributors)
	if err != nil {
		t.Fatalf("ToJSON returned error: %v", err)
	}

	if result == "" {
		t.Error("ToJSON returned empty string")
	}
}

func TestParseCommitsEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		input       []string
		excludeBots bool
		wantCount   int
	}{
		{
			name:        "invalid format",
			input:       []string{"invalid-line"},
			excludeBots: false,
			wantCount:   0,
		},
		{
			name:        "excludes qodana bot",
			input:       []string{"qodana-support@jetbrains.com||Qodana Bot||abc123||2023-01-01"},
			excludeBots: false,
			wantCount:   0,
		},
		{
			name:        "excludes github bot when flag set",
			input:       []string{"dependabot[bot]@users.noreply.github.com||dependabot[bot]||abc123||2023-01-01"},
			excludeBots: true,
			wantCount:   0,
		},
		{
			name:        "includes github bot when flag not set",
			input:       []string{"dependabot[bot]@users.noreply.github.com||dependabot[bot]||abc123||2023-01-01"},
			excludeBots: false,
			wantCount:   1,
		},
		{
			name:        "empty input",
			input:       []string{},
			excludeBots: false,
			wantCount:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commits := parseCommits(tt.input, tt.excludeBots)
			if len(commits) != tt.wantCount {
				t.Errorf("parseCommits() returned %d commits, want %d", len(commits), tt.wantCount)
			}
		})
	}
}
