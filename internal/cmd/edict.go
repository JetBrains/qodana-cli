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

// newEdictInstallCommand returns the command setting Edict up for Codex sessions started in the current directory.
func newEdictInstallCommand() *cobra.Command {
	var deniedPaths []string
	command := &cobra.Command{
		Use:   "install",
		Short: "Set Edict up for Codex sessions in the current directory",
		Long: `Install the managed Edict skills into ./.codex/skills and write ./.codex/config.toml
with the Edict permissions, agent limits, and the edict-mcp server address. The
server port is edict.mcpPort in ./qodana.yaml (or ./qodana.yml), 27182 by default:

  edict:
    mcpPort: 27182

The global $CODEX_HOME is never changed: configure the model provider there before
running this command, and trust the current directory in Codex: Codex ignores
the local config otherwise. The command asks Codex ('codex mcp list', or $CODEX_BIN)
whether it loads the config, and removes it and fails if not. Re-run the command after changing the
port; it updates skills and config. Unrelated files in ./.codex/skills are kept.
Use --deny for paths agents must not read, such as benchmark answers.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			args := []string{"install"}
			for _, path := range deniedPaths {
				args = append(args, "--deny", path)
			}
			return runEdictJVM(command, args...)
		},
	}
	command.Flags().StringArrayVar(&deniedPaths, "deny", nil, "Deny agents any access to this path (relative to the current directory)")
	return command
}
