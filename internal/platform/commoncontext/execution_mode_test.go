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

package commoncontext

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseExecutionMode(t *testing.T) {
	tests := []struct {
		withinDocker string
		expected     ExecutionMode
		expectError  bool
	}{
		{withinDocker: "", expected: ExecutionModeNotSpecified},
		{withinDocker: "true", expected: ExecutionModeDocker},
		{withinDocker: "FALSE", expected: ExecutionModeNative},
		{withinDocker: "maybe", expected: ExecutionModeNotSpecified, expectError: true},
	}
	for _, tt := range tests {
		t.Run(
			tt.withinDocker, func(t *testing.T) {
				got, err := ParseExecutionMode(tt.withinDocker)
				assert.Equal(t, tt.expected, got)
				assert.Equal(t, tt.expectError, err != nil)
			},
		)
	}
}
