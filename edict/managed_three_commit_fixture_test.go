// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package edict

const (
	threeCommitFixtureProject  = "testExtractSignalsFromThreeCommits"
	threeCommitFixtureBaseline = "26b38d1203a6697bac3ec38659f25ed6e6f50f05"
	threeCommitFixtureEquality = "b375279dbaa2dd53d64dd9047bdb596d0d5160e7"
	threeCommitFixtureLocale   = "4a448d1e6e30961c43024f9900f80e6d3d088b79"
	threeCommitFixtureHead     = "19475f69ff6ed87a68712b4ad9d55938f3868b6e"
)

// The baseline is outside HEAD~3..HEAD. Each correction changes line 5 of one
// independent Java file and supplies one positive/negative evidence pair.
func threeCommitSignalExpectations() []managedCommitExpectation {
	var expected []managedCommitExpectation
	for _, commit := range []struct{ parent, revision, file string }{
		{threeCommitFixtureBaseline, threeCommitFixtureEquality, "RoleMatcher.java"},
		{threeCommitFixtureEquality, threeCommitFixtureLocale, "KeyNormalizer.java"},
		{threeCommitFixtureLocale, threeCommitFixtureHead, "TimeoutConverter.java"},
	} {
		expected = append(expected, managedCommitExpectation{
			Parent: commit.parent,
			Commit: commit.revision,
			Path:   threeCommitFixtureProject + "/src/main/java/com/mycompany/app/" + commit.file,
			Evidence: []managedSignalEvidence{
				{Label: "POSITIVE", Revision: commit.parent, Line: 5},
				{Label: "NEGATIVE", Revision: commit.revision, Line: 5},
			},
		})
	}
	return expected
}
