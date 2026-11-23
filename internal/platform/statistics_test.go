package platform

import (
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
	"github.com/stretchr/testify/assert"
)

func TestCommonEventDataHasCloudAndAnalysisIds(t *testing.T) {
	linterInfo := thirdpartyscan.LinterInfo{LinterVersion: "2026.2"}

	assert.Equal(
		t,
		map[string]string{
			"version": "2026.2", qodanaProjectId: "project", qodanaAnalysisId: "0f8fad5b-d9cb-469f-a165-70867728950e",
		},
		commonEventData(linterInfo, "project", "0f8fad5b-d9cb-469f-a165-70867728950e"),
	)
	assert.Equal(t, map[string]string{"version": "2026.2"}, commonEventData(linterInfo, "", ""))
}
