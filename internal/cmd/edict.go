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
	"github.com/spf13/cobra"
)

// newEdictCommand returns the parent command for edict-related tooling.
func newEdictCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edict",
		Short: "Edict commands: extract inspection rules from your development history",
	}
	cmd.AddCommand(newEdictInstallCommand(), newEdictLinterMCPCommand(), newEdictManagedMCPCommand())
	return cmd
}

// newEdictInstallCommand returns the command installing bundled edict skills into the Codex CLI.
func newEdictInstallCommand() *cobra.Command {
	cliOptions := &edictInstallOptions{}
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install managed Edict skills into the local Codex CLI",
		Long: `Install the managed Edict skills bundled in the Kotlin application
into the Codex CLI skills directory, so that 'codex' discovers them automatically.

By default skills are installed user-wide into $CODEX_HOME/skills (~/.codex/skills).
Use --project to install into <project-dir>/.codex/skills instead.
Installs edict_manager and its capability-controlled skill copies by default.
The --managed flag is retained for compatibility.
Existing skill files are overwritten, so re-running the command updates the skills.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			destDir := cliOptions.DestDir
			if destDir == "" {
				var err error
				destDir, err = codexSkillsDirectory(cliOptions.Project, cliOptions.ProjectDir)
				if err != nil {
					return err
				}
			}
			return runEdictJVM(cmd, "install-skills", "--directory", destDir)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&cliOptions.Project, "project", false, "Install into <project-dir>/.codex/skills instead of the user-wide Codex skills directory")
	flags.BoolVar(&cliOptions.Managed, "managed", false, "Compatibility flag; managed skills are always installed")
	flags.StringVarP(&cliOptions.ProjectDir, "project-dir", "i", ".", "Root directory of the project (used with --project)")
	flags.StringVar(&cliOptions.DestDir, "dest", "", "Install into a custom directory (overrides --project and the default location)")
	return cmd
}

type edictInstallOptions struct {
	Managed    bool
	Project    bool
	ProjectDir string
	DestDir    string
}
