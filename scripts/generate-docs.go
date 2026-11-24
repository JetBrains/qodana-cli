//go:build ignore
// +build ignore

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

// Script to generate CLI documentation in README.md using cobra/doc.
// Documents the commands of the README in its order and format: headings without the root command, no SEE ALSO
// sections nor generation dates. A new command is documented once it is added to readmeCommands.
package main

import (
	"bytes"
	"os"
	"regexp"
	"strings"

	"github.com/JetBrains/qodana-cli/internal/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func main() {
	cmd.InitCli()
	root := cmd.GetRootCommand()

	var docs bytes.Buffer
	writeDoc := func(c *cobra.Command) {
		var command bytes.Buffer
		c.DisableAutoGenTag = true
		_ = doc.GenMarkdownCustom(c, &command, func(string) string { return "" })
		markdown, _, _ := strings.Cut(command.String(), "### SEE ALSO")
		docs.WriteString(strings.Replace(markdown, "## "+root.Name()+" ", "## ", 1))
	}

	for _, name := range readmeCommands {
		c, _, err := root.Find([]string{name})
		if err != nil || c == root {
			println("The CLI has no command " + name + " of the README.")
			os.Exit(1)
		}
		writeDoc(c)
	}

	result := cleanup(docs.String())
	content, _ := os.ReadFile("README.md")
	newContent := regexp.MustCompile(`(?s)## init\n.*?(## Why)`).ReplaceAllString(string(content), result+"$1")

	if newContent == string(content) && !strings.Contains(string(content), result) {
		println("README.md has no section from '## init' to '## Why' to put the documentation in.")
		os.Exit(1)
	}
	if len(os.Args) > 1 && os.Args[1] == "--check" {
		if string(content) != newContent {
			println("README.md is out of date. Run 'go run scripts/generate-docs.go' to update it.")
			os.Exit(1)
		}
		println("README.md is up to date.")
		return
	}

	os.WriteFile("README.md", []byte(newContent), 0644)
	println("README.md updated.")
}

// readmeCommands are the commands the README documents, in its order: cobra sorts them by name.
var readmeCommands = []string{"init", "scan", "show", "send", "pull", "view", "contributors", "cloc"}

// cleanup removes ANSI codes, UUID defaults and the blank lines a description starts or ends with, and replaces home
// dir with ~
func cleanup(s string) string {
	s = regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\n{3,}`).ReplaceAllString(s, "\n\n")
	s = regexp.MustCompile(` \(default "[0-9a-f-]{36}"\)`).ReplaceAllString(s, "")
	if h, _ := os.UserHomeDir(); h != "" {
		s = strings.ReplaceAll(s, h, "~")
	}
	return s
}
