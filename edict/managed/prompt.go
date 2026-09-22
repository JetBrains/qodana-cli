// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package managed

import (
	"errors"
	"fmt"
	"strings"
)

// Called with the store locked. Only a child's freshly minted token may enter
// its spawn message; no capability may enter the persisted prompt template.
func (s *Store) validatePrompt(skill, prompt string) error {
	declaration := "$managed-" + skill
	first, body, ok := strings.Cut(prompt, "\n")
	if !ok || first != declaration || strings.TrimSpace(body) == "" {
		return fmt.Errorf("subagent prompt must start with the exact line %q followed by its skill file path and bounded task instructions", declaration)
	}
	for _, candidate := range possibleToken.FindAllString(prompt, -1) {
		if _, secret := s.issuedTokens[hash(candidate)]; secret {
			return errors.New("subagent prompt must not contain capability tokens; credentials are appended by the server")
		}
	}
	return nil
}

func taskLaunchPrompt(taskID, skill, token string) string {
	return fmt.Sprintf(`$managed-%s
Before loading any skill, call edict_task_get with token %s to fetch your assigned task %s. Read the returned prompt and its assigned SKILL.md, then execute that task using the managed lifecycle. Use only the assigned worker skill; do not load edict_manager/SKILL.md, which is for the root manager.`, skill, token, taskID)
}
