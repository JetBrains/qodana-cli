package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// GitAllowFileProtocol allows the use of file:// protocol for the duration of the test.
func GitAllowFileProtocol(t *testing.T) {
	t.Helper()

	// I have to do this bullshit because for some reason git does not allow injecting just a path to git config.
	// If no other source has provided GIT_CONFIG_ stuff before, this will set:
	//   GIT_CONFIG_COUNT=1
	//   GIT_CONFIG_KEY_0=protocol.file.allow
	//   GIT_CONFIG_VALUE_0=always
	// kind of like an array.
	count := 0
	countStr := os.Getenv("GIT_CONFIG_COUNT")
	if countStr != "" {
		if _, err := fmt.Sscanf(countStr, "%d", &count); err != nil {
			t.Fatalf("Scanf failed while parsing the value of GIT_CONFIG_COUNT, which was %s: %s", countStr, err)
		}
	}

	t.Setenv(fmt.Sprintf("GIT_CONFIG_KEY_%d", count), "protocol.file.allow")
	t.Setenv(fmt.Sprintf("GIT_CONFIG_VALUE_%d", count), "always")
	t.Setenv("GIT_CONFIG_COUNT", fmt.Sprintf("%d", count+1))
}

type GitRepo struct {
	t   *testing.T
	dir string
}

func (g *GitRepo) Run(args ...string) string {
	g.t.Helper()
	fullArgs := append(
		[]string{
			"-c", "user.name=Test",
			"-c", "user.email=test@test.com",
			"-c", "commit.gpgsign=false",
			"-c", "tag.gpgsign=false",
			"-c", "protocol.file.allow=always",
		},
		args...,
	)
	cmd := exec.Command("git", fullArgs...)
	cmd.Dir = g.dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		g.t.Fatalf("git %v failed in %s: %v\n  stdout: %s\n  stderr: %s", args, g.dir, err, stdout.String(), stderr.String())
	}
	if stderr.Len() > 0 {
		fmt.Print(stderr.String())
	}
	return strings.TrimSpace(stdout.String())
}

// CommitAll stages all files and creates a commit with the given message.
// Returns the SHA of the created commit.
func (g *GitRepo) CommitAll(message string) string {
	g.t.Helper()
	g.Run("add", ".")
	g.Run("commit", "--allow-empty", "--allow-empty-message", "-m", message)
	return g.RevParse("HEAD")
}

// RevParse returns the SHA for the given ref.
func (g *GitRepo) RevParse(ref string) string {
	g.t.Helper()
	return g.Run("rev-parse", ref)
}

// Dir returns the directory path of the repository.
func (g *GitRepo) Dir() string {
	return g.dir
}

// Tag creates a lightweight tag at the current HEAD.
func (g *GitRepo) Tag(name string) {
	g.t.Helper()
	g.Run("tag", name)
}

// Checkout checks out the given ref.
func (g *GitRepo) Checkout(ref string) {
	g.t.Helper()
	g.Run("checkout", ref)
}

// OriginURL returns the URL of the origin remote.
// Local paths are returned with file:// prefix for compatibility with --depth clones.
func (g *GitRepo) OriginURL() string {
	g.t.Helper()
	url := g.Run("remote", "get-url", "origin")
	if filepath.IsAbs(url) {
		return "file://" + url
	}
	return url
}

// PushAll pushes all branches and tags to origin.
func (g *GitRepo) PushAll() {
	g.t.Helper()
	g.Run("push", "origin", "--all")
	g.Run("push", "origin", "--tags")
}

// Submodule returns a GitRepo for the submodule at the given path.
func (g *GitRepo) Submodule(path string) *GitRepo {
	return &GitRepo{g.t, filepath.Join(g.dir, path)}
}

// AddSubmodule adds a submodule from the given remote URL at the given path.
// The submodule name will be the same as the path.
func (g *GitRepo) AddSubmodule(remote string, path string) *GitRepo {
	g.t.Helper()
	g.Run("submodule", "add", remote, path)
	return g.Submodule(path)
}

// AddSubmoduleWithName adds a submodule with a custom name (different from path).
func (g *GitRepo) AddSubmoduleWithName(remote string, path string, name string) *GitRepo {
	g.t.Helper()
	g.Run("submodule", "add", "--name", name, remote, path)
	return g.Submodule(path)
}

// WriteFile writes content to a file in the repository.
func (g *GitRepo) WriteFile(name string, content string) {
	g.t.Helper()
	err := os.WriteFile(filepath.Join(g.dir, name), []byte(content), 0644)
	if err != nil {
		g.t.Fatalf("Failed to write file %s: %v", name, err)
	}
}

// ReadFile reads a file from the repository.
func (g *GitRepo) ReadFile(name string) string {
	g.t.Helper()
	content, err := os.ReadFile(filepath.Join(g.dir, name))
	if err != nil {
		g.t.Fatalf("Failed to read file %s: %v", name, err)
	}
	return string(content)
}

// GitRepoAt creates a GitRepo for an existing directory (doesn't initialize git).
func GitRepoAt(t *testing.T, dir string) *GitRepo {
	t.Helper()
	return &GitRepo{t: t, dir: dir}
}

// NewGitRepo initializes a new git repository and returns a GitRepo for it.
func NewGitRepo(t *testing.T) *GitRepo {
	t.Helper()
	dir := t.TempDir()
	g := &GitRepo{t: t, dir: dir}
	g.Run("init", "--initial-branch=main")
	g.Run("config", "protocol.file.allow", "always")
	return g
}

func NewBareGitRepo(t *testing.T) *GitRepo {
	t.Helper()
	dir := t.TempDir()
	g := &GitRepo{t: t, dir: dir}
	g.Run("init", "--bare", "--initial-branch=main")
	g.Run("config", "protocol.file.allow", "always")
	return g
}

func NewClonedGitRepo(t *testing.T, origin string) *GitRepo {
	t.Helper()
	dir := t.TempDir()
	g := &GitRepo{t: t, dir: dir}
	g.Run("clone", origin, ".")
	g.Run("config", "protocol.file.allow", "always")
	return g
}

func (g *GitRepo) Clone() *GitRepo {
	g.t.Helper()
	dir := g.t.TempDir()
	clone := &GitRepo{t: g.t, dir: dir}
	clone.Run("clone", "file://"+g.dir, ".")
	clone.Run("config", "protocol.file.allow", "always")
	return clone
}

func (g *GitRepo) CloneShallow() *GitRepo {
	g.t.Helper()
	dir := g.t.TempDir()
	clone := &GitRepo{t: g.t, dir: dir}
	clone.Run("clone", "--depth=1", "file://"+g.dir, ".")
	clone.Run("config", "protocol.file.allow", "always")
	return clone
}

func (g *GitRepo) CloneRecursive() *GitRepo {
	g.t.Helper()
	dir := g.t.TempDir()
	clone := &GitRepo{t: g.t, dir: dir}
	clone.Run("clone", "--recursive", "file://"+g.dir, ".")
	clone.Run("config", "protocol.file.allow", "always")
	return clone
}

// SampleRepo creates a bare repo with two commits tagged v1 and v2.
// v1 contains file.txt with "content-v1", v2 contains "content-v2".
func SampleRepo(t *testing.T) *GitRepo {
	t.Helper()

	origin := NewBareGitRepo(t)
	actual := NewClonedGitRepo(t, origin.Dir())

	actual.WriteFile("file.txt", "content-v1")
	actual.CommitAll("v1")
	actual.Tag("v1")

	actual.WriteFile("file.txt", "content-v2")
	actual.CommitAll("v2")
	actual.Tag("v2")

	actual.PushAll()
	return origin
}

// SampleRepoWithSubmodule creates a bare repo with a submodule at path "submodules/regular" with name "regular".
// The main repo has tags v1 and v2, where v1 has submodule at v1 and v2 has submodule at v2.
func SampleRepoWithSubmodule(t *testing.T) *GitRepo {
	t.Helper()

	subOrigin := SampleRepo(t)

	rootOrigin := NewBareGitRepo(t)
	rootActual := rootOrigin.Clone()
	rootActual.WriteFile("file.txt", "content")
	rootActual.CommitAll("initial")

	// Add submodule with name "regular" at path "submodules/regular"
	subActual := rootActual.AddSubmoduleWithName(subOrigin.Dir(), "submodules/regular", "regular")

	subActual.Checkout("v1")
	rootActual.WriteFile("file.txt", "content-v1")
	rootActual.CommitAll("v1")
	rootActual.Tag("v1")

	subActual.Checkout("v2")
	rootActual.WriteFile("file.txt", "content-v2")
	rootActual.CommitAll("v2")
	rootActual.Tag("v2")

	rootActual.PushAll()
	return rootOrigin
}

// SampleRepoWithFakeSubmodule creates a repo based on SampleRepoWithSubmodule
// with an additional orphaned .gitmodules entry for "submodules/fake" that doesn't exist in git cache.
// This simulates a user manually editing .gitmodules without running git submodule add.
func SampleRepoWithFakeSubmodule(t *testing.T) *GitRepo {
	t.Helper()

	origin := SampleRepoWithSubmodule(t)
	work := origin.Clone()

	// Add a fake submodule entry to .gitmodules without actually adding it to git cache
	gitmodules := work.ReadFile(".gitmodules")
	gitmodules += `
[submodule "fake"]
	path = submodules/fake
	url = https://example.com/fake.git
`
	work.WriteFile(".gitmodules", gitmodules)
	work.CommitAll("add orphaned gitmodules entry")
	work.PushAll()

	return origin
}

// SampleRepoWithOrphanedSubmodule creates a repo based on SampleRepoWithSubmodule
// with an additional submodule "submodules/orphaned" in git cache but removed from .gitmodules.
// This simulates a user removing the .gitmodules entry but not properly removing the submodule.
func SampleRepoWithOrphanedSubmodule(t *testing.T) *GitRepo {
	t.Helper()

	subOrigin := SampleRepo(t)

	origin := SampleRepoWithSubmodule(t)
	work := origin.Clone()

	// Add a second submodule
	orphanedSub := work.AddSubmoduleWithName(subOrigin.Dir(), "submodules/orphaned", "orphaned")
	orphanedSub.Checkout("v1")
	work.CommitAll("add orphaned submodule")

	// Remove the orphaned submodule entry from .gitmodules but keep it in git cache
	work.Run("config", "--file", ".gitmodules", "--remove-section", "submodule.orphaned")
	work.CommitAll("remove orphaned gitmodules entry but keep cache")
	work.PushAll()

	return origin
}
