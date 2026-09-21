// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package edict

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManagedTraceRedactsTokensWithoutHidingArtifactIDs(t *testing.T) {
	manager, worker := strings.Repeat("a", 64), strings.Repeat("b", 64)
	plan, task, digest := strings.Repeat("c", 64), strings.Repeat("d", 64), strings.Repeat("e", 64)
	response := fmt.Sprintf(`{"token":%q,"plan":{"id":%q,"tasks":[{"id":%q}]}}`, manager, plan, task)
	nested, err := json.Marshal(map[string]any{"content": []map[string]string{{"text": response}}})
	require.NoError(t, err)
	trace := string(nested) + fmt.Sprintf("\n{\"arguments\":{\"token\":%q,\"expectedHash\":%q}}\nManager %s; worker %s", worker, digest, manager, worker)
	redacted := redactManagedTrace(trace)
	for _, token := range []string{manager, worker} {
		require.NotContains(t, redacted, token)
	}
	for _, value := range []string{plan, task, digest} {
		require.Contains(t, redacted, value)
	}
	message := "Plan: plans/" + plan + ".json. Token: " + manager
	require.Equal(t, "Plan: plans/"+plan+".json. Token: [redacted]", redactManagedTrace(message, trace))
}
