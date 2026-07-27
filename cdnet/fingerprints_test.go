package main

import (
	"encoding/json"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/sarif"
	"github.com/stretchr/testify/assert"
)

func TestAddQodanaFingerprints(t *testing.T) {
	t.Run(
		"matches Ultimate for a regionless SWEA error", func(t *testing.T) {
			result := regionlessSWEAResult(739)

			addQodanaFingerprints(&result)

			assert.Equal(
				t,
				"3af1a39b03e19f8b988c940169db75bdd64cc087ed5bc93c3cff641f455e77d0",
				result.PartialFingerprints[qodanaFingerprintV1],
			)
			assert.Equal(t, "7c1b504368ae1993", result.PartialFingerprints[qodanaFingerprintV2])
		},
	)

	t.Run(
		"fills each missing version independently", func(t *testing.T) {
			result := regionlessSWEAResult(739)
			result.PartialFingerprints = map[string]string{
				qodanaFingerprintV1: "existing-v1",
			}

			addQodanaFingerprints(&result)

			assert.Equal(t, "existing-v1", result.PartialFingerprints[qodanaFingerprintV1])
			assert.Equal(t, "7c1b504368ae1993", result.PartialFingerprints[qodanaFingerprintV2])
		},
	)

	t.Run(
		"preserves v2 while filling v1", func(t *testing.T) {
			result := regionlessSWEAResult(739)
			result.PartialFingerprints = map[string]string{qodanaFingerprintV2: "existing-v2"}

			addQodanaFingerprints(&result)

			assert.Equal(
				t,
				"3af1a39b03e19f8b988c940169db75bdd64cc087ed5bc93c3cff641f455e77d0",
				result.PartialFingerprints[qodanaFingerprintV1],
			)
			assert.Equal(t, "existing-v2", result.PartialFingerprints[qodanaFingerprintV2])
		},
	)

	t.Run(
		"does not depend on SARIF artifact index", func(t *testing.T) {
			first := regionlessSWEAResult(739)
			second := regionlessSWEAResult(42)
			addQodanaFingerprints(&first)
			addQodanaFingerprints(&second)

			assert.Equal(t, first.PartialFingerprints, second.PartialFingerprints)
		},
	)
}

func TestFingerprint2011MatchesGuava(t *testing.T) {
	tests := []struct {
		length int
		hash   string
	}{
		{0, "e365a64a907cad23"},
		{3, "f991950e842304e1"},
		{32, "bce9f64611a0baf4"},
		{33, "6765cc55861ee437"},
		{64, "8f946c650f0a16a1"},
		{65, "bdadcab83ea71934"},
		{100, "86e5281223241228"},
		{256, "d6d996e1975cdc46"},
	}

	for _, test := range tests {
		data := make([]byte, test.length)
		for i := range data {
			data[i] = byte(i*37 + 11)
		}
		assert.Equal(t, test.hash, fingerprint2011Hex(data), "length %d", test.length)
	}
}

func TestHashThreadFlowPreservesExplicitZeroOrderFields(t *testing.T) {
	tests := []struct {
		name  string
		field string
	}{
		{name: "execution order", field: "executionOrder"},
		{name: "index", field: "index"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ordered := unmarshalThreadFlowLocations(t, `[{"`+test.field+`":0},{"`+test.field+`":1}]`)
			reversed := unmarshalThreadFlowLocations(t, `[{"`+test.field+`":1},{"`+test.field+`":0}]`)

			assert.Equal(t, "0", threadFlowNodeID(ordered[0], 0))
			assert.Equal(t, "1", threadFlowNodeID(ordered[1], 1))

			orderedHasher := &safeHasher{}
			reversedHasher := &safeHasher{}
			hashThreadFlow(orderedHasher, sarif.ThreadFlow{Locations: ordered})
			hashThreadFlow(reversedHasher, sarif.ThreadFlow{Locations: reversed})
			assert.Equal(t, orderedHasher.data, reversedHasher.data)
		})
	}
}

func unmarshalThreadFlowLocations(t *testing.T, data string) []sarif.ThreadFlowLocation {
	t.Helper()
	var locations []sarif.ThreadFlowLocation
	assert.NoError(t, json.Unmarshal([]byte(data), &locations))
	return locations
}

func TestBaselineEqualityMatchesUltimateForRegion(t *testing.T) {
	result := sarif.Result{
		RuleId:  "IgnoreResultOfCall",
		Message: &sarif.Message{Text: "Result of method call ignored"},
		Locations: []sarif.Location{
			{
				PhysicalLocation: &sarif.PhysicalLocation{
					ArtifactLocation: &sarif.ArtifactLocation{Uri: "src/Foo.java", UriBaseId: "SRCROOT"},
					Region: &sarif.Region{
						StartLine:   10,
						StartColumn: 5,
						CharLength:  7,
						Snippet:     &sarif.ArtifactContent{Text: "execute()"},
					},
				},
			},
		},
	}

	assert.Equal(t, "c4226c3153cc0bcb84e7bbd7ffbb874179d95cd8b931850e5984e90a8f955703", baselineEqualityV1(result))
	assert.Equal(t, "24a51315786e59ee", baselineEqualityV2(result))
}

func regionlessSWEAResult(artifactIndex int64) sarif.Result {
	return sarif.Result{
		RuleId:  ".SWEAFileErrors",
		Message: &sarif.Message{Text: "Program 'OmniSharp.dll' does not contain a static 'Main' method suitable for an entry point"},
		Locations: []sarif.Location{
			{
				PhysicalLocation: &sarif.PhysicalLocation{
					ArtifactLocation: &sarif.ArtifactLocation{
						Index:     artifactIndex,
						Uri:       "src/OmniSharp.Stdio.Driver/OmniSharp.Stdio.Driver.csproj",
						UriBaseId: "solutionDir",
					},
				},
			},
		},
	}
}
