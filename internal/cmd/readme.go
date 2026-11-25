/*
 * Copyright 2021-2026 JetBrains s.r.o.
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
	"bytes"
	"fmt"
	"regexp"
	"strings"

	platformcmd "github.com/JetBrains/qodana-cli/internal/platform/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// updateReadmeCommand regenerates the README: see TestReadmeIsUpToDate.
const updateReadmeCommand = "go test ./internal/cmd -run TestReadmeIsUpToDate -update"

// The README documents the commands between these markers.
const (
	readmeCommandsStart = "<!-- Generated from the --help of the commands by '" + updateReadmeCommand +
		"': do not edit. -->\n"
	readmeCommandsEnd = "<!-- End of the generated documentation of the commands. -->\n"
)

var (
	ansiCodes   = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	uuidDefault = regexp.MustCompile(` \(default "[0-9a-f-]{36}"\)`)
)

// generateCliReadme returns readme with the documentation of the commands of the CLI as --help shows them.
func generateCliReadme(readme string) (string, error) {
	root := newRootCommand()
	commands := newChildCommands()
	root.AddCommand(commands...)
	return generateReadme(readme, commands)
}

// generateReadme returns readme with the documentation of commands between the markers, each of which it must have
// once. It documents the available commands, in their order, and then their available subcommands, but the ones with
// platformcmd.UndocumentedAnnotation. It trims the descriptions of the commands it documents.
func generateReadme(readme string, commands []*cobra.Command) (string, error) {
	for _, marker := range []string{readmeCommandsStart, readmeCommandsEnd} {
		if count := strings.Count(readme, marker); count != 1 {
			return "", fmt.Errorf("README.md has the line %q %d times instead of once", marker, count)
		}
	}
	before, rest, _ := strings.Cut(readme, readmeCommandsStart)
	_, after, found := strings.Cut(rest, readmeCommandsEnd)
	if !found {
		return "", fmt.Errorf("README.md has the line %q before the line %q", readmeCommandsEnd, readmeCommandsStart)
	}

	var docs strings.Builder
	if err := writeCommandsMarkdown(&docs, commands); err != nil {
		return "", err
	}
	return before + readmeCommandsStart + "\n" + cleanup(docs.String()) + readmeCommandsEnd + after, nil
}

// writeCommandsMarkdown writes the documentation of the available commands and their available subcommands, but the
// ones with platformcmd.UndocumentedAnnotation.
func writeCommandsMarkdown(docs *strings.Builder, commands []*cobra.Command) error {
	for _, command := range commands {
		if _, undocumented := command.Annotations[platformcmd.UndocumentedAnnotation]; undocumented ||
			!command.IsAvailableCommand() {
			continue
		}
		markdown, err := commandMarkdown(command)
		if err != nil {
			return fmt.Errorf("failed to document the command %q: %w", command.CommandPath(), err)
		}
		docs.WriteString(markdown)
		if err := writeCommandsMarkdown(docs, command.Commands()); err != nil {
			return err
		}
	}
	return nil
}

// commandMarkdown returns the documentation of command in the format of the README: a heading without the root
// command, no SEE ALSO section nor generation date.
func commandMarkdown(command *cobra.Command) (string, error) {
	command.DisableAutoGenTag = true
	// Blank lines that a description starts or ends with would show as more than one blank line.
	command.Short = strings.TrimSpace(command.Short)
	command.Long = strings.TrimSpace(command.Long)

	var markdown bytes.Buffer
	if err := doc.GenMarkdownCustom(command, &markdown, func(string) string { return "" }); err != nil {
		return "", err
	}
	result, _, _ := strings.Cut(markdown.String(), "### SEE ALSO")
	path := command.CommandPath()
	heading := strings.TrimPrefix(path, command.Root().Name()+" ")
	return strings.Replace(result, "## "+path+"\n", "## "+heading+"\n", 1), nil
}

// cleanup removes ANSI codes and UUID defaults, which differ between runs. A help that shows a path of the machine,
// such as the home directory, would differ between machines too: it shows a placeholder such as <userCacheDir> instead.
func cleanup(s string) string {
	s = ansiCodes.ReplaceAllString(s, "")
	return uuidDefault.ReplaceAllString(s, "")
}
