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
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGetContributorsWithMailmap(t *testing.T) {
	repo := newTestRepo(t)

	repo.commit(t, "John Doe", "john@old.com")
	repo.commit(t, "John Doe", "john@new.com")
	repo.commit(t, "J. Doe", "jdoe@personal.com")
	repo.commit(t, "Jane Smith", "jane@company.com")

	if got := len(GetContributors([]string{repo.dir}, -1, false)); got != 4 {
		t.Errorf("Without mailmap: expected 4 contributors, got %d", got)
	}

	repo.writeMailmap(
		t, `John Doe <john@canonical.com> <john@old.com>
John Doe <john@canonical.com> <john@new.com>
John Doe <john@canonical.com> J. Doe <jdoe@personal.com>
`,
	)

	contributors := GetContributors([]string{repo.dir}, -1, false)
	if got := len(contributors); got != 2 {
		t.Errorf("With mailmap: expected 2 contributors, got %d", got)
	}

	johnCommits := countCommitsForEmail(contributors, "john@canonical.com")
	if johnCommits != 3 {
		t.Errorf("Expected John Doe to have 3 commits, got %d", johnCommits)
	}
}

func TestGetContributorsMailmapNameMapping(t *testing.T) {
	repo := newTestRepo(t)

	repo.commit(t, "bobby", "bob@example.com")
	repo.commit(t, "Bob", "bob@example.com")
	repo.writeMailmap(t, "Robert Smith <bob@example.com>\n")

	contributors := GetContributors([]string{repo.dir}, -1, false)

	if contributors[0].Author.Username != "Robert Smith" {
		t.Errorf("Expected canonical name 'Robert Smith', got '%s'", contributors[0].Author.Username)
	}
	if contributors[0].Count != 2 {
		t.Errorf("Expected 2 commits, got %d", contributors[0].Count)
	}
}

type testRepo struct {
	dir     string
	counter int
}

func newTestRepo(t *testing.T) *testRepo {
	dir := t.TempDir()
	runGit(t, dir, nil, "init")
	return &testRepo{dir: dir}
}

func (r *testRepo) commit(t *testing.T, name, email string) {
	r.counter++
	file := filepath.Join(r.dir, "file.txt")
	os.WriteFile(file, []byte{byte(r.counter)}, 0644)
	runGit(t, r.dir, nil, "add", ".")
	runGit(
		t,
		r.dir,
		[]string{
			"GIT_AUTHOR_NAME=" + name,
			"GIT_AUTHOR_EMAIL=" + email,
			"GIT_COMMITTER_NAME=" + name,
			"GIT_COMMITTER_EMAIL=" + email,
		},
		"commit",
		"-m",
		"commit",
	)
}

func (r *testRepo) writeMailmap(t *testing.T, content string) {
	if err := os.WriteFile(filepath.Join(r.dir, ".mailmap"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, dir string, env []string, args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	defaultEnv := []string{
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	}
	cmd.Env = append(os.Environ(), append(defaultEnv, env...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func countCommitsForEmail(contributors []contributor, email string) int {
	for _, c := range contributors {
		if c.Author.Email == email {
			return c.Count
		}
	}
	return 0
}
