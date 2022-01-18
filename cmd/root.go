/*
 * Copyright 2021-2022 JetBrains s.r.o.
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
	"io/ioutil"
	"os"
	"os/signal"

	"github.com/JetBrains/qodana-cli/core"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewRootCmd constructs root command.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "qodana",
		Short:   "Run Qodana CLI",
		Long:    core.Info,
		Version: core.Version,
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
	if err := viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		log.Fatal(err)
	}
	return rootCmd
}

var RootCmd = NewRootCmd()

// init adds all child commands to the root command.
func init() {
	RootCmd.AddCommand(
		NewInitCommand(),
		NewScanCommand(),
		NewShowCommand(),
	)
}

// Execute runs the command, adds Ctrl+C handler and returns error.
func Execute() error {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			core.Interrupted = true
			log.SetOutput(ioutil.Discard)
			core.WarningMessage("Interrupting Qodana CLI...")
			core.DockerCleanup()
			_ = core.QodanaSpinner.Stop()
			os.Exit(0)
		}
	}()
	if err := RootCmd.Execute(); err != nil {
		return err
	}
	return nil
}
