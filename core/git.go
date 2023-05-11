package core

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	log "github.com/sirupsen/logrus"
)

// isGitInstalled checks if git is installed.
func isGitInstalled() bool {
	_, err := exec.LookPath("git")
	if err != nil {
		WarningMessage(
			"Unable to find git, refer to https://git-scm.com/downloads for installing it",
		)
		return false
	}
	return true
}

// gitRun runs the git command in the given directory and returns an error if any.
func gitRun(cwd string, command []string) error {
	cmd := exec.Command("git", command...)
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// gitReset resets the git repository to the given commit.
func gitReset(cwd string, sha string) error {
	return gitRun(cwd, []string{"reset", "--soft", strings.TrimPrefix(sha, "CI")})
}

// gitResetBack aborts the git reset.
func gitResetBack(cwd string) error {
	return gitRun(cwd, []string{"reset", "'HEAD@{1}'"})
}

// gitCheckout checks out the given commit / branch.
func gitCheckout(cwd string, where string) error {
	return gitRun(cwd, []string{"checkout", where})
}

// gitClean cleans the git repository.
func gitClean(cwd string) error {
	return gitRun(cwd, []string{"clean", "-fdx"})
}

// gitOutput runs the git command in the given directory and returns the output.
func gitOutput(cwd string, args []string) []string {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		log.Warn(err.Error())
		return []string{}
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}

// gitLog returns the git log of the given repository in the given format.
func gitLog(cwd string, format string, since int, mailmap bool) []string {
	args := []string{"--no-pager", "log"}
	if format != "" {
		args = append(args, "--pretty=format:"+format)
	}
	if since > 0 {
		args = append(args, fmt.Sprintf("--since=%d.days", since))
	}
	if mailmap {
		args = append(args, "--mailmap")
	}
	return gitOutput(cwd, args)
}

// gitRevisions returns the list of commits of the git repository in chronological order.
func gitRevisions(cwd string) []string {
	return reverse(gitLog(cwd, "%H", 0, false))
}

// gitRemoteUrl returns the remote url of the git repository.
func gitRemoteUrl(cwd string) string {
	return gitOutput(cwd, []string{"remote", "get-url", "origin"})[0]
}

// gitBranch returns the current branch of the git repository.
func gitBranch(cwd string) string {
	return gitOutput(cwd, []string{"rev-parse", "--abbrev-ref", "HEAD"})[0]
}
