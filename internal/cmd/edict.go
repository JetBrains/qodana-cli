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
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// newEdictCommand returns the parent command for edict-related tooling.
func newEdictCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edict",
		Short: "Edict commands: extract inspection rules from your development history",
	}
	cmd.AddCommand(newEdictInstallCommand(), newEdictManagedMCPCommand(), newEdictIDEMCPCommand())
	return cmd
}

// newEdictInstallCommand returns the command installing bundled edict skills into the Codex CLI.
func newEdictInstallCommand() *cobra.Command {
	options := edictInstallOptions{}
	command := &cobra.Command{
		Use:   "install",
		Short: "Install managed Edict skills into the local Codex CLI",
		Long: `Install the managed Edict skills bundled in the Kotlin application
into the Codex CLI skills directory, so that 'codex' discovers them automatically.

By default skills are installed user-wide into $CODEX_HOME/skills (~/.codex/skills).
Use --project to install into ./.codex/skills, --project-dir <dir> to install into
<dir>/.codex/skills, or --dest for any other directory.
Existing skill files are overwritten, so re-running the command updates the skills;
unrelated files in the directory are kept.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			// An explicit --project-dir names a project, so it cannot silently fall back to the user-wide directory.
			options.Project = options.Project || command.Flags().Changed("project-dir")
			destination, err := options.skillsDirectory()
			if err != nil {
				return err
			}
			return runEdictJVM(command, "install-skills", "--directory", destination)
		},
	}
	flags := command.Flags()
	flags.BoolVar(
		&options.Project,
		"project",
		false,
		"Install into the current project's .codex/skills instead of the user-wide Codex skills directory",
	)
	flags.StringVarP(
		&options.ProjectDir,
		"project-dir",
		"i",
		".",
		"Install into <project-dir>/.codex/skills instead of the user-wide Codex skills directory",
	)
	flags.StringVar(
		&options.DestDir,
		"dest",
		"",
		"Install into this directory instead of the user-wide Codex skills directory",
	)
	command.MarkFlagsMutuallyExclusive("project", "dest")
	command.MarkFlagsMutuallyExclusive("project-dir", "dest")
	return command
}

type edictInstallOptions struct {
	Project    bool
	ProjectDir string
	DestDir    string
}

func (options edictInstallOptions) skillsDirectory() (string, error) {
	switch {
	case options.DestDir != "":
		return options.DestDir, nil
	case options.Project:
		project, err := filepath.Abs(options.ProjectDir)
		return filepath.Join(project, ".codex", "skills"), err
	default:
		return codexSkillsDirectory()
	}
}

// codexSkillsDirectory is where Codex discovers user-wide skills.
func codexSkillsDirectory() (string, error) {
	if codexHome := os.Getenv("CODEX_HOME"); codexHome != "" {
		return filepath.Join(codexHome, "skills"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".codex", "skills"), nil
}
