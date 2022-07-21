package core

import (
	"os"
	"os/exec"
	"strings"
)

// IsGitInstalled checks if git is installed.
func IsGitInstalled() bool {
	_, err := exec.LookPath("git")
	if err != nil {
		WarningMessage(
			"Unable to find git, refer to https://git-scm.com/downloads for installing it, no --commit option will be used",
		)
		return false
	}
	return true
}

// GitReset resets the git repository to the given commit.
func git(cwd string, command []string) error {
	cmd := exec.Command("git", command...)
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// GitReset resets the git repository to the given commit.
func GitReset(cwd string, sha string) error {
	return git(cwd, []string{"reset", "--soft", strings.TrimPrefix(sha, "CI")})
}

// gitResetBack aborts the git revert.
func gitResetBack(cwd string) error {
	return git(cwd, []string{"reset", "'HEAD@{1}'"})
}
