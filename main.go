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

package main

import (
	"fmt"
	"os"

	"github.com/JetBrains/qodana-cli/cmd"
	"github.com/JetBrains/qodana-cli/core"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
)

func main() {
	if !core.IsInteractive() || os.Getenv("NO_COLOR") != "" { // http://no-color.org
		pterm.DisableColor()
	}
	if os.Getenv("DO_NOT_TRACK") == "1" { // https://consoledonottrack.com
		core.DoNotTrack = true
	}
	if err := cmd.Execute(); err != nil {
		log.Fatal(fmt.Sprintf("error running command: %s", err))
		os.Exit(1)
	}
}
