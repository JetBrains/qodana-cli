// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package managed

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDelegationRejectsInvalidPromptWithoutChangingTask(t *testing.T) {
	store, manager, _ := testStore(t)
	task := store.Plan().Tasks[0]
	valid := testPrompt(task.Skill)
	for _, prompt := range []string{
		"", "$managed-" + task.Skill, "$managed-" + task.Skill + "\n ",
		"Use " + valid, strings.Replace(valid, "$managed-"+task.Skill, "$edict_manager", 1),
		valid + "\nParent token: " + manager,
	} {
		before := store.Plan()
		_, err := store.Delegate(manager, task.ID, nil, nil, prompt)
		require.Error(t, err)
		require.NotContains(t, err.Error(), manager)
		require.Equal(t, before, store.Plan(), "rejected prompt must not mutate the task")
		require.Len(t, store.grants, 1, "rejected prompt must not mint a child capability")
	}
}

func TestWorkerMustFetchItsAssignmentBeforeStarting(t *testing.T) {
	store, manager, _ := testStore(t)
	task := store.Plan().Tasks[0]
	prompt := testPrompt(task.Skill) + "\nAnalyze commit-19475f69ff6ed87a only.\nPreserve this trailing newline.\n"
	grant, err := store.Delegate(manager, task.ID, []string{"inbox.write"}, []string{"inbox"}, prompt)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(grant.Prompt, "$managed-"+task.Skill+"\n"))
	require.Contains(t, grant.Prompt, "edict_task_get")
	require.NotContains(t, grant.Prompt, "commit-19475f69ff6ed87a", "launch messages must not copy the task instructions")
	require.Equal(t, 1, strings.Count(grant.Prompt, grant.Token))
	before := store.Plan()
	require.Equal(t, prompt, before.Tasks[0].Prompt)
	_, err = store.ReadTask(manager)
	require.Error(t, err, "the manager does not have a worker assignment")
	_, err = store.ReadTask("invalid-token")
	require.Error(t, err)
	_, err = store.StartTask(grant.Token, "native-worker", grant.Skill)
	require.ErrorContains(t, err, "edict_task_get")
	require.Equal(t, before, store.Plan())
	signal := validTestSignal(t, "assignment-read")
	_, err = store.Write(grant.Token, signal.Path, signal.Content, "")
	require.Error(t, err)
	assignment, err := store.ReadTask(grant.Token)
	require.NoError(t, err)
	require.Equal(t, TaskAssignment{TaskID: grant.TaskID, Skill: grant.Skill, SkillPath: grant.SkillPath, Prompt: prompt}, assignment)
	require.Equal(t, before, store.Plan(), "fetching the assignment does not rewrite the plan")
	_, err = store.StartTask(grant.Token, "native-worker", "edict_manager")
	require.ErrorContains(t, err, "task skill mismatch")
	plan, err := store.StartTask(grant.Token, "native-worker", assignment.Skill)
	require.NoError(t, err)
	require.Equal(t, "running", plan.Tasks[0].Status)
	_, err = store.Write(grant.Token, signal.Path, signal.Content, "")
	require.NoError(t, err)
	encoded, err := json.Marshal(plan)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), grant.Token)
	_, err = store.FinishTask(grant.Token, "failed", "Retry with different source inputs")
	require.NoError(t, err)
	_, err = store.ReadTask(grant.Token)
	require.Error(t, err, "revoked workers cannot fetch tasks")
	replacement, err := store.Delegate(manager, task.ID, nil, nil, testPrompt(task.Skill)+"\nInspect another commit.")
	require.NoError(t, err)
	_, err = store.StartTask(replacement.Token, "replacement-worker", replacement.Skill)
	require.ErrorContains(t, err, "edict_task_get", "a retry must fetch its new assignment")
	retry, err := store.ReadTask(replacement.Token)
	require.NoError(t, err)
	require.NotEqual(t, assignment.Prompt, retry.Prompt)
	_, err = store.StartTask(replacement.Token, "replacement-worker", retry.Skill)
	require.NoError(t, err)
}
