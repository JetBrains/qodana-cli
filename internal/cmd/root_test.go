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
	"testing"

	edictcmd "github.com/JetBrains/qodana-cli/edict/cmd"
)

func TestEdictServerCommandDetection(t *testing.T) {
	root := newRootCommand()
	root.AddCommand(edictcmd.NewCommand())
	for _, args := range [][]string{
		{"edict", "mcp", "start"},
		{"--log-level", "debug", "edict", "mcp", "start"},
		{"edict", "--disable-update-checks", "mcp", "start", "--config", "/tmp/qodana.yaml"},
		{"edict", "ide-mcp", "--project-dir", "/tmp/project"},
	} {
		if !isEdictServerCommand(root, args) {
			t.Errorf("Edict server invocation was not detected: %v", args)
		}
	}
	for _, args := range [][]string{
		{"edict", "install", "--deny", "mcp"},
		{"edict", "mcp"},
		{"edict"},
	} {
		if isEdictServerCommand(root, args) {
			t.Errorf("ordinary CLI command was mistaken for an Edict server: %v", args)
		}
	}
}
