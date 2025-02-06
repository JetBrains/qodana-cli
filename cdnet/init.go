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

package main

import (
	"github.com/JetBrains/qodana-cli/v2024/cmd"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	platformcmd "github.com/JetBrains/qodana-cli/v2024/platform/cmd"
	"github.com/JetBrains/qodana-cli/v2024/platform/thirdpartyscan"
	"github.com/spf13/cobra"
)

func Execute(productCode string, linterName string, linterVersion string, buildDateStr string, isEap bool) {
	platform.CheckEAP(buildDateStr, isEap)

	linter := CdnetLinter{}

	linterInfo := thirdpartyscan.LinterInfo{
		ProductCode:   productCode,
		LinterName:    linterName,
		LinterVersion: linterVersion,
		IsEap:         isEap,
	}

	commands := make([]*cobra.Command, 1)
	commands[0] = platformcmd.NewScanCommand(linter, linterInfo)
	cmd.InitWithCustomCommands(commands)
	cmd.Execute()
}
