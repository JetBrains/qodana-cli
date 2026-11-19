/*
 * Copyright 2026 JetBrains s.r.o.
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

package cmd

import (
	"errors"
	"runtime"

	"github.com/spf13/cobra"
)

// NewCommand returns the parent command for edict-related tooling.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edict",
		Short: "Edict commands: extract inspection rules from your development history",
	}
	cmd.AddCommand(
		newEdictInstallCommand(),
		newEdictManagedMCPCommand(),
		newEdictIDEMCPCommand(),
		newEdictStateCommand(),
		newEdictFeedbackCommand(),
	)
	return cmd
}

func requireEdictRoutinePlatform(_ *cobra.Command, _ []string) error {
	if runtime.GOOS == "darwin" && runtime.GOARCH == "amd64" {
		return errors.New("the Edict daily routine is unavailable on x86-64 macOS: ONNX Runtime no longer ships native libraries for this platform; use a supported platform such as Apple Silicon with the arm64 Qodana CLI")
	}
	return nil
}

// newEdictFeedbackCommand returns the commands a CI run uses to show findings of not yet promoted Edict
// inspections on reviews: a GitHub Actions run of a pull request, or a TeamCity build of a Space SafePush.
func newEdictFeedbackCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "feedback",
		Short: "Show findings of not yet promoted Edict inspections on reviews",
	}
	command.AddCommand(newEdictFeedbackPrepareCommand(), newEdictFeedbackPostCommand())
	return command
}

// newEdictFeedbackPrepareCommand returns the command that runs before the scan.
func newEdictFeedbackPrepareCommand() *cobra.Command {
	var stateDir, outputDir, configFile string
	prepare := &cobra.Command{
		Use:   "prepare",
		Short: "Prepare the Edict inspections to run on a review",
		Long: `In a GitHub Actions run for a pull request between branches of the repository, or in a TeamCity
build of a SafePush when edict.ci is on Space, check the Edict state branch (edict.stateBranch, edict/state by
default) out at --state-dir, unless CI checked the state out there, and copy into --output-dir,
outside the project and missing or empty, the generated inspections whose findings may become review comments: the
most precise of the not yet promoted ones. Pass the directory to 'qodana scan' as
--property qodana.internal.inspections.kts.dir=<directory>, which runs them without reporting their findings.

Copies nothing when the run gets no feedback: it is for no pull request or for one from a fork, for no
SafePush or one of bots only, the review already got all its feedback, or no inspection qualifies.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			args := append(
				[]string{"feedback-prepare", "--state-dir", stateDir, "--output-dir", outputDir},
				edictQodanaYamlArguments(".", configFile)...,
			)
			return runEdictJVM(command, args...)
		},
	}
	prepare.Flags().StringVar(&stateDir, "state-dir", "", "Directory to check the Edict state out at")
	_ = prepare.MarkFlagRequired("state-dir")
	prepare.Flags().StringVar(&outputDir, "output-dir", "", "Missing or empty directory outside the project to copy the inspections to run into")
	_ = prepare.MarkFlagRequired("output-dir")
	prepare.Flags().StringVar(&configFile, "config", "", "Qodana configuration file (defaults to qodana.yml or qodana.yaml in the current directory)")
	return prepare
}

// newEdictFeedbackPostCommand returns the command that runs after the scan.
func newEdictFeedbackPostCommand() *cobra.Command {
	var stateDir, sarifFile, configFile string
	post := &cobra.Command{
		Use:   "post",
		Short: "Show a review the findings of the inspections 'prepare' selected",
		Long: `After 'qodana scan' ran the inspections of 'feedback prepare', reserve their findings in the SARIF
report in what the review changes, within the limits of the review and of each inspection, by pushing them to
the Edict state branch from --state-dir, the checkout of 'prepare'. Then show them at the commit the report
names as analysed, asking for an answer.

On GitHub, comments on the lines the pull request adds: reads the pull request from GITHUB_EVENT_PATH and the
API from GITHUB_API_URL, and comments with GITHUB_TOKEN (or GH_TOKEN), which needs to write pull requests.
On Space, notifies the author of the SafePush through the Edict Slack service of the findings in the files
their commits change, once a week per inspection and lines: reads the build from TEAMCITY_BUILD_PROPERTIES_FILE,
finds its merge request with SPACE_TOKEN and notifies with EDICT_NOTIFICATION_TOKEN.
Pushing the state needs the checkout's credentials to write contents.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			args := append(
				[]string{"feedback-post", "--state-dir", stateDir, "--sarif", sarifFile},
				edictQodanaYamlArguments(".", configFile)...,
			)
			return runEdictJVM(command, args...)
		},
	}
	post.Flags().StringVar(&stateDir, "state-dir", "", "Directory where 'feedback prepare' checked the Edict state out")
	_ = post.MarkFlagRequired("state-dir")
	post.Flags().StringVar(&sarifFile, "sarif", "", "SARIF report of the scan, qodana.sarif.json in its results directory")
	_ = post.MarkFlagRequired("sarif")
	post.Flags().StringVar(&configFile, "config", "", "Qodana configuration file (defaults to qodana.yml or qodana.yaml in the current directory)")
	return post
}

// newEdictStateCommand returns the commands for the Edict state branch that install checks out.
func newEdictStateCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "state",
		Short: "Manage the Edict state branch",
	}
	var message, configFile string
	push := &cobra.Command{
		Use:   "push",
		Short: "Commit the Edict state and push its branch to origin",
		Long: `Commit every change in the Edict state (edict.statePath, ./.edict by default) on its branch
(edict.stateBranch, edict/state by default) and push the branch to origin, setting it as upstream.
If another run pushed the branch first, the commit is rebased onto it. Fails while an Edict server
owns the state: stop it with 'qodana edict mcp stop' first.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			args := append([]string{"state-push"}, edictQodanaYamlArguments(".", configFile)...)
			if message != "" {
				args = append(args, "--message", message)
			}
			return runEdictJVM(command, args...)
		},
	}
	push.Flags().StringVar(&message, "message", "", "Commit message (default \"Update the Edict state\")")
	push.Flags().StringVar(&configFile, "config", "", "Qodana configuration file (defaults to qodana.yml or qodana.yaml in the current directory)")
	command.AddCommand(push)
	return command
}

// newEdictInstallCommand returns the command setting Edict up for Codex sessions started in the current directory.
func newEdictInstallCommand() *cobra.Command {
	var deniedPaths []string
	var configFile string
	command := &cobra.Command{
		Use:   "install",
		Short: "Set Edict up for Codex sessions in the current directory",
		Long: `Install the managed Edict skills into ./.codex/skills and add to ./.codex/config.toml
the Edict permissions, agent limits, and the edict-mcp server address. The
server port is edict.mcpPort in ./qodana.yaml (or ./qodana.yml), 27182 by default:

  edict:
    mcpPort: 27182

The global $CODEX_HOME is never changed: configure the model provider there before
running this command, and trust the current directory in Codex: Codex ignores
the local config otherwise. The command asks Codex ('codex mcp list', or $CODEX_BIN)
whether it loads the config, and restores the previous config and fails if not. Re-run the command
after changing the port; it updates skills and config. The lines it writes end with '# qodana-edict'
and are replaced on each run; every other line and unrelated files in ./.codex/skills are kept. The
command fails without changes when the config sets a value Edict needs, such as approval_policy, differently.
Agents may not read Edict's process logs or the sessions and logs in $CODEX_HOME; use --deny for
other paths agents must not read, such as benchmark answers.

In a git repository, install then checks out the Edict state branch (edict.stateBranch, edict/state
by default) as a worktree at edict.statePath, from origin when it has the branch, else as a new
branch without history, and excludes it from the project's git status. The project's checkout stays
on its branch. A state directory that already holds files is kept as it is.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		PreRunE:      requireEdictRoutinePlatform,
		RunE: func(command *cobra.Command, _ []string) error {
			args := append([]string{"install"}, edictQodanaYamlArguments(".", configFile)...)
			for _, path := range deniedPaths {
				args = append(args, "--deny", path)
			}
			return runEdictJVM(command, args...)
		},
	}
	command.Flags().StringArrayVar(&deniedPaths, "deny", nil, "Deny agents any access to this path (relative to the current directory)")
	command.Flags().StringVar(&configFile, "config", "", "Qodana configuration file (defaults to qodana.yml or qodana.yaml in the current directory)")
	return command
}
