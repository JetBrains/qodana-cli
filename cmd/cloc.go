/*
 * Copyright 2021-2023 JetBrains s.r.o.
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
	"github.com/boyter/scc/v3/processor"
	"github.com/spf13/cobra"
)

// clocOptions represents contributor command options.
type clocOptions struct {
	ProjectDirs []string
	Output      string
}

// newClocCommand returns a new instance of the show command.
func newClocCommand() *cobra.Command {
	options := &clocOptions{}
	cmd := &cobra.Command{
		Use:   "cloc",
		Short: "Calculate lines of code for projects with boyter/scc",
		Long:  `A command-line helper for project statistics: languages, lines of code. Powered by boyter/scc. For contributors, use "qodana contributors" command.`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(options.ProjectDirs) == 0 {
				options.ProjectDirs = append(options.ProjectDirs, ".")
			}
			processor.Format = options.Output
			processor.Cocomo = true
			processor.DirFilePaths = options.ProjectDirs
			if processor.ConfigureLimits != nil {
				processor.ConfigureLimits()
			}
			processor.ConfigureGc()
			processor.ConfigureLazy(true)
			processor.Process()
		},
	}
	flags := cmd.Flags()
	flags.StringArrayVarP(&options.ProjectDirs, "project-dir", "i", []string{}, "Project directory, can be specified multiple times to check multiple projects, if not specified, current directory will be used")
	flags.StringVarP(&options.Output, "output", "o", "tabular", "Output format, can be [tabular, wide, json, csv, csv-stream, cloc-yaml, html, html-table, sql, sql-insert, openmetrics]")

	return cmd
}
