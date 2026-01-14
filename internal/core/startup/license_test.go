package startup

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAllCommunityNames(t *testing.T) {
	result := allCommunityNames()

	assert.NotEmpty(t, result)
	assert.Contains(t, result, "Qodana")
	assert.True(t, strings.Contains(result, ",") || strings.Contains(result, "\""))
}


