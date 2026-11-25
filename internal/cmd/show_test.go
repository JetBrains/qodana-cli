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

package cmd

import (
	"errors"
	"fmt"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/commoncontext"
	"github.com/JetBrains/qodana-cli/internal/platform/telemetry"
	"github.com/stretchr/testify/assert"
)

func TestShowFailure(t *testing.T) {
	assert.Equal(t, "", showFailure(nil))
	assert.Equal(t, telemetry.ReasonReportMissing, showFailure(commoncontext.ErrReportMissing))
	assert.Equal(t, telemetry.ReasonReportMissing, showFailure(fmt.Errorf("show: %w", commoncontext.ErrReportMissing)))
	assert.Equal(t, telemetry.ReasonServeFailed, showFailure(errors.New("address already in use")))
}

// The statistics are off in this package, so the test only shows that every outcome of ShowReport is taken.
func TestReportShown(t *testing.T) {
	for _, target := range []string{"cloud", "local", "dir"} {
		reportShown(target, nil)
	}
	reportShown("local", commoncontext.ErrReportMissing)
	reportShown("local", errors.New("address already in use"))
}
