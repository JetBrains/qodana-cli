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
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	platformcmd "github.com/JetBrains/qodana-cli/internal/platform/cmd"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var updateReadme = flag.Bool("update", false, "regenerate README.md instead of checking it: "+updateReadmeCommand)

// TestReadmeIsUpToDate checks that README.md documents the commands as their --help shows them, and regenerates it
// with -update.
func TestReadmeIsUpToDate(t *testing.T) {
	const path = "../../README.md"
	readme, err := os.ReadFile(path)
	require.NoError(t, err)

	updated, err := generateCliReadme(string(readme))

	require.NoError(t, err)
	if *updateReadme {
		require.NoError(t, os.WriteFile(path, []byte(updated), 0o644))
		return
	}
	// Not assert.Equal, which would print the whole README twice before the difference.
	if line, readmeLine, updatedLine, differ := firstDifferentLine(string(readme), updated); differ {
		t.Fatalf("README.md is out of date: run '%s'. Its line %d is\n%q\nbut --help shows\n%q",
			updateReadmeCommand, line, readmeLine, updatedLine)
	}
}

// firstDifferentLine returns the number of the first line in which a and b differ, and the lines: past the end of the
// shorter text, the rest of the longer one and nothing.
func firstDifferentLine(a, b string) (line int, aLine, bLine string, differ bool) {
	aLines, bLines := strings.Split(a, "\n"), strings.Split(b, "\n")
	common := min(len(aLines), len(bLines))
	for i := range common {
		if aLines[i] != bLines[i] {
			return i + 1, aLines[i], bLines[i], true
		}
	}
	return common + 1, strings.Join(aLines[common:], "\n"), strings.Join(bLines[common:], "\n"), len(aLines) != len(bLines)
}

func TestGenerateReadmeDocumentsTheAvailableCommandsInTheirOrder(t *testing.T) {
	commands := newReadmeTestCommands(
		newRunnableTestCommand("second", "\nThe second command\n"),
		newRunnableTestCommand("first", "The first command"),
		&cobra.Command{Use: "hidden", Short: "A hidden command", Hidden: true, Run: func(*cobra.Command, []string) {}},
		&cobra.Command{Use: "group", Short: "A command without an available subcommand"},
		newUndocumentedTestCommand("internal", "not publicly available"),
	)

	updated, err := generateReadme(readmeWithCommands("outdated\n"), commands)

	require.NoError(t, err)
	assert.Equal(t, readmeWithCommands(testCommandMarkdown("second", "The second command")+
		testCommandMarkdown("first", "The first command")), updated)
}

func TestGenerateReadmeDocumentsTheAvailableSubcommandsAfterTheirCommand(t *testing.T) {
	group := &cobra.Command{Use: "group", Short: "A group of commands"}
	group.AddCommand(
		newRunnableTestCommand("run", "Run something"),
		newUndocumentedTestCommand("internal", "not publicly available"),
	)
	commands := newReadmeTestCommands(group, newRunnableTestCommand("next", "The next command"))

	updated, err := generateReadme(readmeWithCommands(""), commands)

	require.NoError(t, err)
	docs := strings.TrimPrefix(strings.TrimSuffix(updated, readmeCommandsEnd), readmeCommandsStart)
	var headings []string
	for _, line := range strings.Split(docs, "\n") {
		if strings.HasPrefix(line, "## ") {
			headings = append(headings, line)
		}
	}
	assert.Equal(t, []string{"## group", "## group run", "## next"}, headings)
	assert.Contains(t, docs, "\nqodana group run [flags]\n")
}

func TestGenerateReadmeKeepsTheTextAroundTheCommandsAsIs(t *testing.T) {
	commands := newReadmeTestCommands(newRunnableTestCommand("only", "Costs $1 and ${HOME}"))
	readme := "# $1 ${HOME}\n" + readmeWithCommands("") + "## Why $1\n"

	updated, err := generateReadme(readme, commands)

	require.NoError(t, err)
	docs := readmeWithCommands(testCommandMarkdown("only", "Costs $1 and ${HOME}"))
	assert.Equal(t, "# $1 ${HOME}\n"+docs+"## Why $1\n", updated)
}

func TestGenerateReadmeFailsWithoutEachMarkerOnce(t *testing.T) {
	timesInsteadOfOnce := func(marker string, count int) string {
		return fmt.Sprintf("README.md has the line %q %d times instead of once", marker, count)
	}
	for name, test := range map[string]struct{ readme, err string }{
		"no markers": {"## init\n\n## Why\n", timesInsteadOfOnce(readmeCommandsStart, 0)},
		"no start":   {"## init\n" + readmeCommandsEnd, timesInsteadOfOnce(readmeCommandsStart, 0)},
		"no end":     {readmeCommandsStart + "## init\n", timesInsteadOfOnce(readmeCommandsEnd, 0)},
		"two starts": {readmeWithCommands(readmeCommandsStart), timesInsteadOfOnce(readmeCommandsStart, 2)},
		"two ends":   {readmeWithCommands(readmeCommandsEnd), timesInsteadOfOnce(readmeCommandsEnd, 2)},
		"swapped": {
			readmeCommandsEnd + readmeCommandsStart,
			fmt.Sprintf("README.md has the line %q before the line %q", readmeCommandsEnd, readmeCommandsStart),
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := generateReadme(test.readme, nil)

			assert.EqualError(t, err, test.err)
		})
	}
}

func newReadmeTestCommands(commands ...*cobra.Command) []*cobra.Command {
	root := &cobra.Command{Use: "qodana"}
	root.AddCommand(commands...)
	return commands
}

func newRunnableTestCommand(name, short string) *cobra.Command {
	return &cobra.Command{Use: name, Short: short, Run: func(*cobra.Command, []string) {}}
}

func newUndocumentedTestCommand(name, reason string) *cobra.Command {
	command := newRunnableTestCommand(name, "A command the README does not document")
	command.Annotations = map[string]string{platformcmd.UndocumentedAnnotation: reason}
	return command
}

func testCommandMarkdown(name, short string) string {
	return "## " + name + "\n\n" + short + "\n\n```\nqodana " + name + " [flags]\n```\n\n" +
		"### Options\n\n```\n  -h, --help[=true|false]   help for " + name + "\n```\n\n"
}

// readmeWithCommands returns the section of the README with docs, which follow a blank line as in README.md.
func readmeWithCommands(docs string) string {
	return readmeCommandsStart + "\n" + docs + readmeCommandsEnd
}
