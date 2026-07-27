package main

import (
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFingerprintsAgainstNativeQodanaFixture(t *testing.T) {
	report, err := platform.ReadReport("../test_linter/testdata/mocked-results/qodana.sarif.json")
	require.NoError(t, err)
	checked := 0
	for _, run := range report.Runs {
		for _, result := range run.Results {
			v1 := result.PartialFingerprints[qodanaFingerprintV1]
			v2 := result.PartialFingerprints[qodanaFingerprintV2]
			if v1 == "" || v2 == "" {
				continue
			}
			assert.Equal(t, v1, baselineEqualityV1(result), result.RuleId)
			assert.Equal(t, v2, baselineEqualityV2(result), result.RuleId)
			checked++
		}
	}
	assert.Positive(t, checked)
}
