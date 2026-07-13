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

package cmd

import (
	"strings"

	"github.com/JetBrains/qodana-cli/edict"
	"github.com/JetBrains/qodana-cli/internal/platform/msg"
	"github.com/spf13/cobra"
)

// newEdictCommand returns the parent command for edict-related tooling.
func newEdictCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edict",
		Short: "Edict commands: extract inspection rules from your development history",
	}
	cmd.AddCommand(newEdictSetupCodexCommand())
	return cmd
}

// newEdictSetupCodexCommand returns the command installing bundled edict skills into the Codex CLI.
func newEdictSetupCodexCommand() *cobra.Command {
	cliOptions := &edictSetupCodexOptions{}
	cmd := &cobra.Command{
		Use:   "setup-codex",
		Short: "Install edict skills into the local Codex CLI",
		Long: `Install the edict agent skills bundled with the Qodana CLI (e.g. extract-review-signals)
into the Codex CLI skills directory, so that 'codex' discovers them automatically.

By default skills are installed user-wide into $CODEX_HOME/skills (~/.codex/skills).
Use --project to install into <project-dir>/.codex/skills instead.
Existing skill files are overwritten, so re-running the command updates the skills.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			destDir := cliOptions.DestDir
			if destDir == "" {
				var err error
				destDir, err = edict.CodexSkillsDir(cliOptions.Project, cliOptions.ProjectDir)
				if err != nil {
					return err
				}
			}
			installed, err := edict.InstallSkills(destDir)
			if err != nil {
				return err
			}
			msg.SuccessMessage("Installed %d skill(s) into %s: %s", len(installed), destDir, strings.Join(installed, ", "))
			return nil
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&cliOptions.Project, "project", false, "Install into <project-dir>/.codex/skills instead of the user-wide Codex skills directory")
	flags.StringVarP(&cliOptions.ProjectDir, "project-dir", "i", ".", "Root directory of the project (used with --project)")
	flags.StringVar(&cliOptions.DestDir, "dest", "", "Install into a custom directory (overrides --project and the default location)")
	return cmd
}

type edictSetupCodexOptions struct {
	Project    bool
	ProjectDir string
	DestDir    string
}
