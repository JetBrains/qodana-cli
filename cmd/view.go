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
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/JetBrains/qodana-cli/v2024/platform/platforminit"
	"github.com/spf13/cobra"
)

// viewOptions represents view command options.
type viewOptions struct {
	SarifFile string
}

// newViewCommand returns a new instance of the show command.
func newViewCommand() *cobra.Command {
	options := &viewOptions{}
	cmd := &cobra.Command{
		Use:   "view",
		Short: "View SARIF files in CLI",
		Long:  `Preview all problems found in SARIF files in CLI.`,
		Run: func(cmd *cobra.Command, args []string) {
			platform.ProcessSarif(options.SarifFile, "", "", true, false, false)
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&options.SarifFile, "sarif-file", "f", platforminit.QodanaSarifName, "Path to the SARIF file")
	return cmd
}
