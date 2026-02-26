/*
 * Copyright 2021-2024 JetBrains s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package platformcmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func parseScanOptionsForTest(t *testing.T, args ...string) CliOptions {
	t.Helper()

	var options CliOptions
	cmd := &cobra.Command{Use: "scan"}
	err := ComputeFlags(cmd, &options)
	assert.NoError(t, err)

	err = cmd.ParseFlags(args)
	assert.NoError(t, err)

	return options
}

func TestGetShowReportPortParsing(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected int
	}{
		{
			name:     "show-report-port defined",
			args:     []string{"--show-report-port", "9001"},
			expected: 9001,
		},
		{
			name:     "both defined show-report-port wins",
			args:     []string{"--port", "9002", "--show-report-port", "9003"},
			expected: 9003,
		},
		{
			name:     "only port defined",
			args:     []string{"--port", "9004"},
			expected: 9004,
		},
		{
			name:     "no flags uses default 8080",
			args:     []string{},
			expected: 8080,
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name, func(t *testing.T) {
				options := parseScanOptionsForTest(t, tc.args...)
				assert.Equal(t, tc.expected, options.GetShowReportPort())
			},
		)
	}
}
