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
// Documents init and scan first, then all other non-hidden commands.
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

	cmdMap := make(map[string]*cobra.Command)
	for _, c := range root.Commands() {
		cmdMap[c.Name()] = c
	}

	var docs bytes.Buffer
	writeDoc := func(c *cobra.Command) {
		doc.GenMarkdownCustom(c, &docs, func(string) string { return "" })
		docs.WriteString("\n")
	}

	if c := cmdMap["init"]; c != nil && !c.Hidden {
		writeDoc(c)
	}
	if c := cmdMap["scan"]; c != nil && !c.Hidden {
		writeDoc(c)
	}
	for _, c := range root.Commands() {
		if !c.Hidden && c.Name() != "init" && c.Name() != "scan" {
			writeDoc(c)
		}
	}

	result := cleanup(docs.String())
	content, _ := os.ReadFile("README.md")
	newContent := regexp.MustCompile(`(?s)## qodana init\n.*?(## Why)`).ReplaceAllString(string(content), result+"$1")

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

// cleanup removes ANSI codes, UUID defaults, and replaces home dir with ~
func cleanup(s string) string {
	s = regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(s, "")
	s = regexp.MustCompile(` \(default "[0-9a-f-]{36}"\)`).ReplaceAllString(s, "")
	if h, _ := os.UserHomeDir(); h != "" {
		s = strings.ReplaceAll(s, h, "~")
	}
	return s
}
