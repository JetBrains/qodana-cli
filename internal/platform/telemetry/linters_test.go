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

// The package of the test imports product, which imports telemetry.
package telemetry_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	fus "github.com/JetBrains/fus-reporting-api-go"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinterEnumCoversProducts(t *testing.T) {
	file, err := os.Open("fus/events-scheme.json")
	require.NoError(t, err)
	defer func() { _ = file.Close() }()
	var scheme fus.EventsScheme
	require.NoError(t, json.NewDecoder(file).Decode(&scheme))

	checked := 0
	for _, group := range scheme.Scheme {
		for _, event := range group.Schema {
			for _, field := range event.Fields {
				if field.Path != "linter" {
					continue
				}
				checked++
				require.Len(t, field.Value, 1)
				codes := strings.Split(strings.TrimSuffix(strings.TrimPrefix(field.Value[0], "{enum:"), "}"), "|")
				for _, linter := range product.AllLinters {
					assert.Contains(t, codes, linter.ProductCode,
						"%s/%s: a new linter is a change of the registered scheme", group.ID, event.Event)
				}
			}
		}
	}
	assert.Equal(t, 6, checked, "linter.selected, linter.ready, linter.failed, docker.failed and the analysis results")
}
