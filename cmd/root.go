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
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/core"
	"github.com/JetBrains/qodana-cli/v2024/platform/msg"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdenv"
	"github.com/JetBrains/qodana-cli/v2024/platform/version"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
)

// isHelpOrVersion checks if only help was requested.
func isHelpOrVersion(args []string) bool {
	return len(args) == 2 && (args[1] == "--help" || args[1] == "-h" || args[1] == "help" || args[1] == "--version" || args[1] == "-v")
}

func isCompletionRequested(args []string) bool {
	return len(args) >= 2 && args[1] == "completion"
}

// isCommandRequested checks if any command is requested.
func isCommandRequested(commands []*cobra.Command, args []string) string {
	for _, c := range commands {
		for _, a := range args {
			if c.Name() == a {
				return c.Name()
			}
		}
	}
	return ""
}

// setDefaultCommandIfNeeded sets default scan command if no other command is requested.
func setDefaultCommandIfNeeded(rootCmd *cobra.Command, args []string) {
	if !(isHelpOrVersion(args) || isCommandRequested(
		rootCmd.Commands(),
		args[1:],
	) != "" || isCompletionRequested(args)) {
		newArgs := append([]string{"scan"}, args[1:]...)
		rootCmd.SetArgs(newArgs)
	}
}

// Execute is a main CLI entrypoint: handles user interrupt, CLI start and everything else.
func Execute() {
	if !qdenv.IsContainer() && os.Geteuid() == 0 {
		msg.WarningMessage("Running the tool as root is dangerous: please run it as a regular user")
	}
	go core.CheckForUpdates(version.Version)
	if !msg.IsInteractive() || os.Getenv("NO_COLOR") != "" { // http://no-color.org
		msg.DisableColor()
	}

	setDefaultCommandIfNeeded(rootCommand, os.Args)
	if err := rootCommand.Execute(); err != nil {
		core.CheckForUpdates(version.Version)
		_, err = fmt.Fprintf(os.Stderr, "error running command: %s\n", err)
		if err != nil {
			return
		}
		os.Exit(1)
	}

	core.CheckForUpdates(version.Version)
}

// newRootCommand constructs root command.
func newRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "qodana",
		Short:   "Run Qodana CLI",
		Long:    msg.InfoString(version.Version),
		Version: version.Version,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logLevel, err := log.ParseLevel(viper.GetString("log-level"))
			if err != nil {
				log.Fatal(err)
			}
			log.SetLevel(logLevel)
		},
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				err := cmd.Help()
				if err != nil {
					return
				}
			}
		},
	}
	rootCmd.PersistentFlags().String("log-level", "error", "Set log-level for output")
	rootCmd.PersistentFlags().BoolVar(
		&core.DisableCheckUpdates,
		"disable-update-checks",
		false,
		"Disable check for updates",
	)
	if err := viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		log.Fatal(err)
	}
	return rootCmd
}

var rootCommand = newRootCommand()

// InitCli adds all child commands to the root command.
func InitCli() {
	rootCommand.AddCommand(
		newInitCommand(),
		newScanCommand(),
		newShowCommand(),
		newSendCommand(),
		newPullCommand(),
		newViewCommand(),
		newContributorsCommand(),
		newClocCommand(),
	)
}

// InitWithCustomCommands adds custom commands to the root command.
func InitWithCustomCommands(commands []*cobra.Command) {
	rootCommand.AddCommand(commands...)
}
