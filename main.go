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
	"os"

	"github.com/JetBrains/qodana-cli/cmd"
	"github.com/JetBrains/qodana-cli/core"
	log "github.com/sirupsen/logrus"
)

func main() {
	if os.Getenv("DO_NOT_CHECK_UPDATE") != "" {
		core.CheckForUpdates(core.Version)
	}
	if os.Getenv("DO_NOT_TRACK") != "" { // https://consoledonottrack.com
		core.DoNotTrack = true
	}
	if !core.IsInteractive() || os.Getenv("NO_COLOR") != "" { // http://no-color.org
		core.DisableColor()
	}
	if err := cmd.Execute(); err != nil {
		log.Fatalf("error running command: %s", err)
		os.Exit(1)
	}
}
