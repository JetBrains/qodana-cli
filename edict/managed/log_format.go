// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package managed

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

const readableLogWidth = 120

// Agent messages and MCP activity share one format. Continuations are indented
// instead of repeating the timestamp and caller on every line. Count Unicode
// characters, preserve existing indentation, and split long paths when needed.
func formatAgentRecord(at time.Time, task Task, kind, text string) string {
	skill := displaySkill(task.Skill)
	prefix := fmt.Sprintf("%s [%s/%s] %s: ", at.Local().Format("2006/01/02 15:04:05"), skill, shortTaskID(task.ID), kind)
	var record strings.Builder
	text = strings.ReplaceAll(text, "\t", "    ")
	for _, line := range strings.Split(strings.TrimRight(text, "\r\n"), "\n") {
		runes := []rune(strings.TrimSuffix(line, "\r"))
		for {
			width := readableLogWidth - len([]rune(prefix))
			cut := len(runes)
			if cut > width {
				cut = width
				for i := width - 1; i > 0; i-- {
					if unicode.IsSpace(runes[i]) && strings.TrimSpace(string(runes[:i])) != "" {
						cut = i + 1
						break
					}
				}
			}
			record.WriteString(prefix)
			record.WriteString(string(runes[:cut]))
			record.WriteByte('\n')
			prefix = "    "
			runes = runes[cut:]
			if len(runes) == 0 {
				break
			}
		}
	}
	return record.String()
}
