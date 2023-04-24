package core

import (
	"os"
	"os/exec"
	"strings"
)

// isGitInstalled checks if git is installed.
func isGitInstalled() bool {
	_, err := exec.LookPath("git")
	if err != nil {
		WarningMessage(
			"Unable to find git, refer to https://git-scm.com/downloads for installing it, no --commit option will be used",
		)
		return false
	}
	return true
}

// gitReset resets the git repository to the given commit.
func git(cwd string, command []string) error {
	cmd := exec.Command("git", command...)
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// gitReset resets the git repository to the given commit.
func gitReset(cwd string, sha string) error {
	return git(cwd, []string{"reset", "--soft", strings.TrimPrefix(sha, "CI")})
}

// gitResetBack aborts the git revert.
func gitResetBack(cwd string) error {
	return git(cwd, []string{"reset", "'HEAD@{1}'"})
}

// gitCheckout checks out the given commit / branch.
func gitCheckout(cwd string, where string) error {
	return git(cwd, []string{"checkout", where})
}

// gitClean cleans the git repository.
func gitClean(cwd string) error {
	return git(cwd, []string{"clean", "-fdx"})
}

func gitRemoteUrl(cwd string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitBranch returns the current branch of the git repository.
func gitBranch(cwd string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitRevisions returns the list of commits of the git repository in chronological order.
func gitRevisions(cwd string) ([]string, error) {
	cmd := exec.Command("git", "log", "--pretty=format:%H")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	revisions := strings.Split(string(out), "\n")
	for i, j := 0, len(revisions)-1; i < j; i, j = i+1, j-1 {
		revisions[i], revisions[j] = revisions[j], revisions[i]
	}

	return revisions, nil
}
