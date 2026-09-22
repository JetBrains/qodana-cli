package managed

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func testStore(t *testing.T) (*Store, string, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	created, err := s.CreatePlan("Test managed work", []Step{{Skill: "edict-next-batch-signal-analysis", Title: "Analyze signals"}})
	require.NoError(t, err)
	return s, created.Token, dir
}

func testPrompt(skill string) string {
	return "$managed-" + skill + "\nRead /skills/managed-" + skill + "/SKILL.md. Inspect the assigned fixture."
}

func delegateTestTask(s *Store, token, taskID string, ops, scope []string) (Delegation, error) {
	task := findTask(s.Plan(), taskID)
	grant, err := s.Delegate(token, taskID, ops, scope, testPrompt(task.Skill))
	if err == nil {
		_, err = s.ReadTask(grant.Token)
	}
	return grant, err
}

func worker(t *testing.T, s *Store, parent, skill string, ops, scope []string) Delegation {
	t.Helper()
	task, err := s.AddTask(parent, skill, "Execute "+skill)
	require.NoError(t, err)
	d, err := delegateTestTask(s, parent, task.ID, ops, scope)
	require.NoError(t, err)
	_, err = s.StartTask(d.Token, "agent-"+task.ID, d.Skill)
	require.NoError(t, err)
	return d
}

func TestPlanCreationCanOnlyClaimManagerOnce(t *testing.T) {
	s, err := NewStore(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	require.Empty(t, s.grants)
	steps := []Step{{Skill: "edict-next-run", Title: "Process inbox"}}
	for _, invalid := range []struct {
		request string
		steps   []Step
	}{
		{"", steps},
		{"Request", nil},
		{"Request", []Step{{Skill: "unregistered", Title: "Invalid skill"}}},
	} {
		created, err := s.CreatePlan(invalid.request, invalid.steps)
		require.Error(t, err)
		require.Nil(t, created)
		require.Nil(t, s.Plan())
		require.Empty(t, s.grants)
	}

	// Failed persistence must not consume the sole manager claim either.
	require.NoError(t, os.WriteFile(filepath.Join(s.root.Name(), "plans"), []byte("blocked"), 0o600))
	created, err := s.CreatePlan("Request", steps)
	require.Error(t, err)
	require.Nil(t, created)
	require.Empty(t, s.grants)
	require.NoError(t, os.Remove(filepath.Join(s.root.Name(), "plans")))

	created, err = s.CreatePlan("Request", steps)
	require.NoError(t, err)
	require.Len(t, created.Token, 64)
	before := s.Plan()
	rejected, err := s.CreatePlan("Request", steps)
	require.ErrorContains(t, err, "already succeeded")
	require.Nil(t, rejected)
	require.Equal(t, before, s.Plan())
	require.Len(t, s.grants, 1)
	child, err := delegateTestTask(s, created.Token, before.Tasks[0].ID, nil, nil)
	require.NoError(t, err)
	_, err = s.StartTask(child.Token, "worker", child.Skill)
	require.NoError(t, err)
	_, err = s.FinishTask(child.Token, "completed", "Done")
	require.NoError(t, err)
	finished := s.Plan()
	rejected, err = s.CreatePlan("Another request", steps)
	require.ErrorContains(t, err, "already succeeded")
	require.Nil(t, rejected)
	require.Equal(t, finished, s.Plan())
	// The returned token is not part of any persisted or publicly readable plan.
	persisted, err := s.Read("plans/" + finished.ID + ".json")
	require.NoError(t, err)
	require.NotContains(t, persisted.Content, created.Token)

	require.NoError(t, s.Close())
	rejected, err = s.CreatePlan("Closed", steps)
	require.ErrorContains(t, err, "closed")
	require.Nil(t, rejected)

	// A fresh server can start the next plan, preserving historical results.
	reopened, err := NewStore(s.root.Name())
	require.NoError(t, err)
	defer reopened.Close()
	next, err := reopened.CreatePlan("Another request", steps)
	require.NoError(t, err)
	require.NotEqual(t, finished.ID, next.Plan.ID)
	require.NotEqual(t, created.Token, next.Token)
	history, err := reopened.Read("plans/" + finished.ID + ".json")
	require.NoError(t, err)
	require.Equal(t, persisted, history)
}

func TestTaskStartRequiresAssignedSkill(t *testing.T) {
	s, manager, _ := testStore(t)
	task := s.Plan().Tasks[0]
	grant, err := delegateTestTask(s, manager, task.ID, nil, nil)
	require.NoError(t, err)
	before := s.Plan()
	for _, skill := range []string{"", "edict_manager", "edict-next-signal-analysis", "managed-" + task.Skill} {
		_, err := s.StartTask(grant.Token, "worker", skill)
		require.ErrorContains(t, err, "task skill mismatch")
		require.ErrorContains(t, err, task.Skill)
		require.Equal(t, before, s.Plan(), "wrong skill must leave the delegated task unchanged")
	}
	_, err = s.StartTask(grant.Token, "worker", grant.Skill)
	require.NoError(t, err)
}

func TestConcurrentPlanCreationHasOneManager(t *testing.T) {
	s, err := NewStore(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	const callers = 16
	results := make(chan *PlanCreation, callers)
	errors := make(chan error, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			created, err := s.CreatePlan("Concurrent request", []Step{{Skill: "edict-next-run", Title: "Process inbox"}})
			results <- created
			errors <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errors)
	successes := 0
	for created := range results {
		if created != nil {
			successes++
			require.Len(t, created.Token, 64)
			require.Equal(t, s.Plan(), created.Plan)
		}
	}
	require.Equal(t, 1, successes)
	failures := 0
	for err := range errors {
		if err != nil {
			failures++
			require.ErrorContains(t, err, "already succeeded")
		}
	}
	require.Equal(t, callers-1, failures)
	require.Len(t, s.grants, 1)
}

func TestCapabilityAttenuationAndOwnership(t *testing.T) {
	s, manager, _ := testStore(t)
	signal := validTestSignal(t, "allowed")
	_, err := s.Write(manager, "inbox/s-test.json", `{}`, "")
	require.ErrorContains(t, err, "cannot modify")
	_, err = s.AddTask(manager, "unmanaged-skill", "Arbitrary work")
	require.Error(t, err)
	_, err = s.AddTask(manager, "edict-next-code-example", "Bypass manager pipeline")
	require.Error(t, err)
	task := s.Plan().Tasks[0]
	_, err = delegateTestTask(s, manager, task.ID, []string{"inspection.write"}, []string{"inspections"})
	require.Error(t, err)
	batch, err := delegateTestTask(s, manager, task.ID, []string{"inbox.write"}, []string{signal.Path})
	require.NoError(t, err)
	_, err = s.Write(batch.Token, signal.Path, signal.Content, "")
	require.ErrorContains(t, err, "start")
	_, err = s.StartTask(batch.Token, "batch-agent", batch.Skill)
	require.NoError(t, err)
	_, err = delegateTestTask(s, manager, task.ID, nil, nil)
	require.Error(t, err)
	_, err = s.Write(batch.Token, "inbox/s-other.json", `{}`, "")
	require.Error(t, err)
	_, err = s.Write(batch.Token, signal.Path, signal.Content, "")
	require.NoError(t, err)
	leafTask, err := s.AddTask(batch.Token, "edict-next-signal-analysis", "Inspect exact commit")
	require.NoError(t, err)
	_, err = delegateTestTask(s, manager, leafTask.ID, nil, nil)
	require.ErrorContains(t, err, "direct child")
	_, err = delegateTestTask(s, batch.Token, leafTask.ID, nil, []string{"inbox"})
	require.ErrorContains(t, err, "widen")
	_, err = delegateTestTask(s, batch.Token, leafTask.ID, []string{"inbox.write"}, []string{signal.Path})
	require.Error(t, err)
	leaf, err := delegateTestTask(s, batch.Token, leafTask.ID, nil, nil)
	require.NoError(t, err)
	require.NotEqual(t, leaf.Token, batch.Token)
	_, err = s.StartTask(leaf.Token, "batch-agent", leaf.Skill)
	require.ErrorContains(t, err, "fresh subagent")
	_, err = s.StartTask(leaf.Token, "inspection-agent", leaf.Skill)
	require.NoError(t, err)
	_, err = s.Write(leaf.Token, signal.Path, signal.Content, "")
	require.Error(t, err)
	_, err = s.FinishTask(batch.Token, "completed", "Premature success")
	require.ErrorContains(t, err, "subtasks")
	_, err = s.FinishTask(leaf.Token, "completed", "Inspected all evidence")
	require.NoError(t, err)
	_, err = s.FinishTask(batch.Token, "completed", "Persisted evidence")
	require.NoError(t, err)
	_, err = s.Write(batch.Token, signal.Path, signal.Content, "")
	require.ErrorContains(t, err, "revoked")
	_, err = s.AddTask(leaf.Token, "edict-next-signal-analysis", "Replay")
	require.ErrorContains(t, err, "revoked")
}

func TestNarrowedParentCannotRegainOperations(t *testing.T) {
	s, manager, _ := testStore(t)
	run := worker(t, s, manager, "edict-next-run", []string{"cluster.write"}, []string{"clusters/one"})
	generation := worker(t, s, run.Token, "edict-next-generation", []string{"cluster.write"}, []string{"clusters/one"})
	task, err := s.AddTask(generation.Token, "edict-next-cluster-generation", "Generate one")
	require.NoError(t, err)
	_, err = delegateTestTask(s, generation.Token, task.ID, []string{"inspection.write"}, []string{"clusters/one"})
	require.ErrorContains(t, err, "operation")
	_, err = delegateTestTask(s, generation.Token, task.ID, []string{"cluster.write"}, []string{"clusters/one-other"})
	require.ErrorContains(t, err, "scope")
	child, err := delegateTestTask(s, generation.Token, task.ID, []string{"cluster.write"}, []string{"clusters/one"})
	require.NoError(t, err)
	_, err = s.StartTask(child.Token, "cluster-agent", child.Skill)
	require.NoError(t, err)
	_, err = s.Write(child.Token, "clusters/one/description.json", `{"id":"two"}`, "")
	require.ErrorContains(t, err, "ID")
	_, err = s.Write(child.Token, "clusters/one/description.json", `{"id":"one"}`, "")
	require.NoError(t, err)
	_, err = s.Write(generation.Token, "clusters/one/history.md", "Cannot write as coordinator", "")
	require.Error(t, err)
}

func TestExampleWorkerCanOnlyLinkAnExistingSignal(t *testing.T) {
	s, manager, _ := testStore(t)
	ops := []string{"cluster.signal.write", "example.write"}
	scope := []string{"clusters/rule"}
	run := worker(t, s, manager, "edict-next-run", ops, scope)
	distribution := worker(t, s, run.Token, "edict-next-distribution", []string{"cluster.signal.write"}, scope)
	signal := validTestSignal(t, "example-link")
	before := strings.TrimSuffix(signal.Content, "}") + `,"syntheticExampleId":null}`
	file, err := s.Write(distribution.Token, "clusters/rule/signals/"+filepath.Base(signal.Path), before, "")
	require.NoError(t, err)
	generation := worker(t, s, run.Token, "edict-next-generation", ops, scope)
	cluster := worker(t, s, generation.Token, "edict-next-cluster-generation", ops, scope)
	example := worker(t, s, cluster.Token, "edict-next-code-example", ops, scope)
	_, err = s.Write(example.Token, file.Path, strings.Replace(before, `"POSITIVE"`, `"NEGATIVE"`, 1), file.Hash)
	require.Error(t, err)
	_, err = s.Write(example.Token, "clusters/rule/signals/s-new.json", before, "")
	require.Error(t, err)
	require.Error(t, s.Delete(example.Token, file.Path, file.Hash))
	after := strings.Replace(before, `"syntheticExampleId":null`, `"syntheticExampleId":"example-one"`, 1)
	linked, err := s.Write(example.Token, file.Path, after, file.Hash)
	require.NoError(t, err)
	require.NotEqual(t, file.Hash, linked.Hash)
	_, err = s.Write(example.Token, "clusters/rule/synthetic-examples/example-one/metadata.json", `{"id":"example-one"}`, "")
	require.NoError(t, err)
	_, err = s.Write(example.Token, "clusters/rule/description.json", `{"id":"rule"}`, "")
	require.Error(t, err)
}

func TestArtifactContainmentAndOptimisticConcurrency(t *testing.T) {
	s, manager, directory := testStore(t)
	signal := validTestSignal(t, "concurrency")
	changed := strings.Replace(signal.Content, "Wait for completion", "Use synchronization", 1)
	batch := worker(t, s, manager, "edict-next-batch-signal-analysis", []string{"inbox.write"}, []string{"inbox"})
	for _, name := range []string{"../outside.json", "/inbox/escape.json", "inbox/../plans/plan.json", "inbox\\escape.json", "inbox/a:b.json", "inbox/a.json/", "inbox//a.json", "inbox/a.json.", "inbox/S-ONE.json", "plans/current.json", ".edict-mcp-current", ".git/config"} {
		t.Run(name, func(t *testing.T) { _, err := s.Write(batch.Token, name, `{}`, ""); require.Error(t, err) })
	}
	_, err := s.Write(batch.Token, "inbox/s-one.json", "not json", "")
	require.Error(t, err)
	f, err := s.Write(batch.Token, signal.Path, signal.Content, "")
	require.NoError(t, err)
	_, err = s.Write(batch.Token, f.Path, changed, "")
	require.ErrorContains(t, err, "conflict")
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Write(batch.Token, f.Path, changed, f.Hash)
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	successes := 0
	for err := range errors {
		if err == nil {
			successes++
		}
	}
	require.Equal(t, 1, successes)
	outside := filepath.Join(t.TempDir(), "secret.json")
	require.NoError(t, os.WriteFile(outside, []byte(`{"secret":true}`), 0o600))
	if err := os.Symlink(outside, filepath.Join(directory, "inbox", "s-link.json")); err != nil {
		t.Skip("symlinks unavailable")
	}
	_, err = s.Read("inbox/s-link.json")
	require.Error(t, err)
	_, err = s.Write(batch.Token, "inbox/s-link.json", `{}`, "")
	require.Error(t, err)
	_, err = s.List("inbox")
	require.Error(t, err)
	data, err := os.ReadFile(outside)
	require.NoError(t, err)
	require.Equal(t, `{"secret":true}`, string(data))
}

func TestPlansPersistAndResumeWithoutOldCapabilities(t *testing.T) {
	s, manager, directory := testStore(t)
	batchTask := s.Plan().Tasks[0]
	batch, err := delegateTestTask(s, manager, batchTask.ID, []string{"inbox.write"}, []string{"inbox"})
	require.NoError(t, err)
	_, err = s.StartTask(batch.Token, "before-restart-agent", batch.Skill)
	require.NoError(t, err)
	leaf := worker(t, s, batch.Token, "edict-next-signal-analysis", nil, nil)
	_, err = s.FinishTask(leaf.Token, "completed", "Evidence inspected")
	require.NoError(t, err)
	p := s.Plan()
	persisted, err := os.ReadFile(filepath.Join(directory, "plans", p.ID+".json"))
	require.NoError(t, err)
	var disk Plan
	require.NoError(t, json.Unmarshal(persisted, &disk))
	require.Equal(t, *p, disk)
	for _, secret := range []string{manager, batch.Token, leaf.Token} {
		require.NotContains(t, string(persisted), secret)
	}
	other, err := NewStore(directory)
	if other != nil {
		_ = other.Close()
	}
	require.Error(t, err)
	require.NoError(t, s.Close())
	reopened, err := NewStore(directory)
	require.NoError(t, err)
	defer reopened.Close()
	_, err = reopened.CreatePlan("Unrelated request", []Step{{Skill: "edict-next-run", Title: "Process inbox"}})
	require.ErrorContains(t, err, "resume")
	created, err := reopened.CreatePlan(p.Request, []Step{{Skill: p.Tasks[0].Skill, Title: p.Tasks[0].Title}})
	require.NoError(t, err)
	require.Equal(t, reopened.Plan(), created.Plan)
	newManager := created.Token
	require.NotEqual(t, manager, newManager)
	_, err = reopened.AddTask(manager, "edict-next-run", "Replay old manager")
	require.ErrorContains(t, err, "revoked")
	require.Equal(t, "pending", reopened.Plan().Tasks[0].Status)
	require.Equal(t, "completed", reopened.Plan().Tasks[1].Status)
	require.Greater(t, reopened.Plan().Revision, p.Revision)
	_, err = reopened.Write(batch.Token, "inbox/s-replay.json", `{}`, "")
	require.ErrorContains(t, err, "revoked")
	resumed, err := delegateTestTask(reopened, newManager, batchTask.ID, []string{"inbox.write"}, []string{"inbox"})
	require.NoError(t, err)
	_, err = reopened.StartTask(resumed.Token, "new-agent", resumed.Skill)
	require.NoError(t, err)
	_, err = reopened.FinishTask(resumed.Token, "completed", "Resumed and persisted")
	require.NoError(t, err)
	_, err = reopened.CreatePlan("Next run", []Step{{Skill: "edict-next-run", Title: "Process inbox"}})
	require.ErrorContains(t, err, "already succeeded")
	_, err = reopened.Read("plans/" + p.ID + ".json")
	require.NoError(t, err)
}

func TestCoordinatorCancellationRevokesSubtree(t *testing.T) {
	s, manager, _ := testStore(t)
	batch := worker(t, s, manager, "edict-next-batch-signal-analysis", []string{"inbox.write"}, []string{"inbox"})
	leaf := worker(t, s, batch.Token, "edict-next-signal-analysis", nil, nil)
	_, err := s.CancelTask(manager, leaf.TaskID, "Not my direct child")
	require.Error(t, err)
	_, err = s.CancelTask(manager, batch.TaskID, "Worker crashed")
	require.NoError(t, err)
	_, err = s.Write(batch.Token, "inbox/s-late.json", `{}`, "")
	require.ErrorContains(t, err, "revoked")
	_, err = s.FinishTask(leaf.Token, "completed", "Late reply")
	require.ErrorContains(t, err, "revoked")
	for _, task := range s.Plan().Tasks[1:] {
		require.Equal(t, "failed", task.Status)
	}
	retried, err := delegateTestTask(s, manager, batch.TaskID, nil, nil)
	require.NoError(t, err)
	_, err = s.StartTask(retried.Token, "replacement-agent", retried.Skill)
	require.NoError(t, err)
	_, err = s.FinishTask(retried.Token, "completed", "Cannot skip failed child")
	require.Error(t, err)
}

func TestOversizedPlanMutationPreservesRecoverableState(t *testing.T) {
	s, manager, directory := testStore(t)
	task := s.Plan().Tasks[0]
	batch, err := delegateTestTask(s, manager, task.ID, nil, nil)
	require.NoError(t, err)
	_, err = s.StartTask(batch.Token, "agent", batch.Skill)
	require.NoError(t, err)
	before := s.Plan()
	_, err = s.FinishTask(batch.Token, "completed", strings.Repeat("x", maxFileSize))
	require.ErrorContains(t, err, "exceeds 8 MiB")
	require.Equal(t, before, s.Plan())
	disk, err := s.Read("plans/" + before.ID + ".json")
	require.NoError(t, err)
	var persisted Plan
	require.NoError(t, json.Unmarshal([]byte(disk.Content), &persisted))
	require.Equal(t, *before, persisted)
	require.NoError(t, s.Close())
	reopened, err := NewStore(directory)
	require.NoError(t, err)
	defer reopened.Close()
	require.Equal(t, "pending", reopened.Plan().Tasks[0].Status)
}
