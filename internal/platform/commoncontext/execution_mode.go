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
	"fmt"
	"strings"
)

// ExecutionMode is where the analysis was requested to run, set by --within-docker or `withinDocker:` in qodana.yaml.
type ExecutionMode int

const (
	// ExecutionModeNotSpecified leaves the choice to the defaults: Docker for an explicit linter, any for auto-detection.
	ExecutionModeNotSpecified ExecutionMode = iota
	ExecutionModeDocker
	ExecutionModeNative
)

// ParseExecutionMode parses a --within-docker / `withinDocker:` value: "true", "false" (any case) or empty.
func ParseExecutionMode(withinDocker string) (ExecutionMode, error) {
	switch strings.ToLower(withinDocker) {
	case "":
		return ExecutionModeNotSpecified, nil
	case "true":
		return ExecutionModeDocker, nil
	case "false":
		return ExecutionModeNative, nil
	}
	return ExecutionModeNotSpecified, fmt.Errorf("unknown within-docker value %q, use true or false", withinDocker)
}

// description says where the mode runs the analysis and which within-docker value requests it, for messages.
func (m ExecutionMode) description() string {
	switch m {
	case ExecutionModeDocker:
		return "in Docker (within-docker=true)"
	case ExecutionModeNative:
		return "natively (within-docker=false)"
	default:
		return "in Docker or natively (within-docker not specified)"
	}
}
