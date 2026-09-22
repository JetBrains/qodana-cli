// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package edict

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JetBrains/qodana-cli/edict/managed"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedEdictExtractClusterAndGenerate(t *testing.T) {
	if testing.Short() {
		t.Skip("real Codex and IntelliJ generation is long")
	}
	requireCodexProvider(t)
	ultimate := requireUltimateEdictRepository(t)
	// The latest commit fixes int overflow in a seconds-to-milliseconds
	// conversion. This project has no seeded clusters, examples, or inspections.
	project := prepareManagedTestProject(t, threeCommitFixtureHead, threeCommitFixtureProject)
	expected := threeCommitSignalExpectations()[2]
	inspection := startManagedInspectionServer(t, project, ultimate)
	codex := prepareManagedCodexWithInspectionServer(t, project, inspection.URL)

	result := codex.run(t, "Run three tasks in order: extract signals from the latest commit, cluster them, then generate inspections.", 40*time.Minute)

	codex.assertCompletedTasks(t, result,
		"edict-next-batch-signal-analysis", "edict-next-signal-analysis", "edict-next-distribution",
		"edict-next-generation", "edict-next-cluster-generation", "edict-next-code-example",
		"edict-next-inspection-code-review", "edict-next-weak-signal-review", "edict-next-inspection-value-review")
	assertManagedSequentialStages(t, project, "edict-next-batch-signal-analysis", "edict-next-distribution", "edict-next-generation")
	assertManagedGeneratedCluster(t, project, inspection, expected)
	assertManagedCheckoutUnchanged(t, project)
}

func assertManagedSequentialStages(t *testing.T, project managedTestProject, skills ...string) {
	t.Helper()
	var stages []managed.Task
	for _, task := range project.Store.Plan().Tasks {
		if task.ParentID == "" {
			stages = append(stages, task)
		}
	}
	require.Len(t, stages, len(skills), "pipeline must have exactly the requested top-level stages")
	var ids []string
	for i, task := range stages {
		assert.Equal(t, skills[i], task.Skill, "pipeline stage %d", i+1)
		ids = append(ids, task.ID)
	}
	log, err := os.Open(filepath.Join(project.Checkout.TestRoot, "log", "edict", "edict-mcp-system.log"))
	require.NoError(t, err)
	defer log.Close()
	problems, err := managedStageOrderProblems(log, ids)
	require.NoError(t, err)
	for _, problem := range problems {
		t.Error(problem)
	}
}

func assertManagedGeneratedCluster(t *testing.T, project managedTestProject, inspection managedInspectionServer, expected managedCommitExpectation) {
	t.Helper()
	inbox, err := project.Store.List("inbox")
	require.NoError(t, err)
	assert.Empty(t, inbox, "clustering must consume every extracted signal")
	descriptions, err := filepath.Glob(filepath.Join(project.StateDirectory, "clusters", "*", "description.json"))
	require.NoError(t, err)
	require.Len(t, descriptions, 1, "the positive/negative pair must form one new cluster")
	var description struct {
		ID, Language, Status string
		PredecessorID        *string `json:"predecessorId"`
	}
	require.NoError(t, json.Unmarshal(mustReadFile(t, descriptions[0]), &description))
	cluster := filepath.Base(filepath.Dir(descriptions[0]))
	assert.Equal(t, cluster, description.ID)
	assert.Equal(t, "Java", description.Language)
	require.Equal(t, "Generated", description.Status, "generation must produce an accepted inspection; see cluster history and agent logs")
	assert.Nil(t, description.PredecessorID)
	signalPaths, err := filepath.Glob(filepath.Join(filepath.Dir(descriptions[0]), "signals", "*.json"))
	require.NoError(t, err)
	var signals []managedCommitSignalFile
	for _, path := range signalPaths {
		var signal managedCommitSignal
		require.NoError(t, json.Unmarshal(mustReadFile(t, path), &signal))
		signals = append(signals, managedCommitSignalFile{Name: "clusters/" + cluster + "/signals/" + filepath.Base(path), Signal: signal})
	}
	assertManagedCommitSignalFiles(t, project.Checkout, expected, signals)
	accepted := "inspections/" + cluster + ".inspection.kts"
	code := string(mustReadFile(t, filepath.Join(project.StateDirectory, accepted)))
	require.NotEmpty(t, strings.TrimSpace(code))
	inspectionPaths, err := project.Store.List("inspections")
	require.NoError(t, err)
	assert.Equal(t, []string{accepted}, inspectionPaths, "generation must leave only the accepted inspection, with no candidate")
	assert.NotEmpty(t, strings.TrimSpace(string(mustReadFile(t, filepath.Join(filepath.Dir(descriptions[0]), "history.md")))))

	// Recompile the persisted bytes and check the original Git evidence using
	// the real IDE, independently of any success claimed by the model.
	contextPath := strings.TrimPrefix(expected.Path, threeCommitFixtureProject+"/")
	for _, evidence := range expected.Evidence {
		source := runCommand(t, project.Checkout.RepositoryDirectory, project.Checkout.GitBinary, "show", evidence.Revision+":"+expected.Path)
		measured := inspection.run(t, code, contextPath, source)
		if evidence.Label == "POSITIVE" {
			assert.True(t, slices.ContainsFunc(measured.FoundProblems, func(p managedInspectionProblem) bool {
				return p.LineNumber == evidence.Line
			}), "accepted inspection must detect the original overflow on line %d", evidence.Line)
		} else {
			assert.Empty(t, measured.FoundProblems, "accepted inspection must not flag the corrected revision")
		}
	}
	for _, file := range signals {
		signal := file.Signal
		require.NotNil(t, signal.SyntheticExampleID, "%s: generation must assign an example", file.Name)
		exampleDir := filepath.Join(filepath.Dir(descriptions[0]), "synthetic-examples", *signal.SyntheticExampleID)
		var metadata struct {
			ID, FileName, Label string
			ExpectedRanges      []historicalLineRange
		}
		require.NoError(t, json.Unmarshal(mustReadFile(t, filepath.Join(exampleDir, "metadata.json")), &metadata))
		assert.Equal(t, *signal.SyntheticExampleID, metadata.ID)
		assert.Equal(t, signal.Label, metadata.Label)
		require.Equal(t, filepath.Base(metadata.FileName), metadata.FileName, "example must use a single source file")
		source := string(mustReadFile(t, filepath.Join(exampleDir, "project", metadata.FileName)))
		measured := inspection.run(t, code, contextPath, source)
		if signal.Label == "POSITIVE" {
			require.Len(t, metadata.ExpectedRanges, 1)
			require.Len(t, measured.FoundProblems, 1, "positive example must contain exactly one detected problem")
			assert.GreaterOrEqual(t, measured.FoundProblems[0].LineNumber, metadata.ExpectedRanges[0].Start)
			assert.LessOrEqual(t, measured.FoundProblems[0].LineNumber, metadata.ExpectedRanges[0].End)
		} else {
			assert.Empty(t, metadata.ExpectedRanges)
			assert.Empty(t, measured.FoundProblems, "negative example must not be reported")
		}
	}
	assertManagedGenerationEvidence(t, project.Checkout.TestRoot, code)
}

func assertManagedGenerationEvidence(t *testing.T, root, code string) {
	t.Helper()
	digest := sha256.Sum256([]byte(code))
	hash := hex.EncodeToString(digest[:])
	acceptedReviews := 0
	require.NoError(t, filepath.WalkDir(filepath.Join(root, "scratch"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}
		var review struct{ CandidateHash, Status string }
		if json.Unmarshal(mustReadFile(t, path), &review) == nil && review.CandidateHash == hash && review.Status == "ACCEPT" {
			acceptedReviews++
		}
		return nil
	}))
	assert.GreaterOrEqual(t, acceptedReviews, 2, "code and value reviews must accept the exact persisted inspection hash")
	file, err := os.Open(filepath.Join(root, "log", "inspection-mcp.jsonl"))
	require.NoError(t, err)
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 16<<20)
	measured := 0
	for scanner.Scan() {
		var call struct {
			Tool      string
			Arguments struct{ InspectionKtsCode string }
			Result    *mcp.CallToolResult
		}
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &call))
		if call.Tool == "run_inspection_kts" && call.Arguments.InspectionKtsCode == code {
			result, err := decodeManagedInspectionResult(call.Result)
			if err == nil && result.CompilationSuccess {
				measured++
			}
		}
	}
	require.NoError(t, scanner.Err())
	assert.GreaterOrEqual(t, measured, 3, "the agent must compile and measure its final candidate on examples and project source")
}
