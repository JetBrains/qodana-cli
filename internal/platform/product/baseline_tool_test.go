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

package product

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCloudBaselineToolName(t *testing.T) {
	for _, tc := range []struct {
		productCode string
		toolName    string
	}{
		// every JVM linter reports itself as QDJVM, so one baseline serves them all
		{QDJVM, QDJVM},
		{QDJVMC, QDJVM},
		{QDAND, QDJVM},
		{QDANDC, QDJVM},
		{QDJVMC + EapSuffix, QDJVM},
		// the other linters report the product code the CLI knows them by
		{QDNET, QDNET},
		{QDNETC, QDNETC},
		{QDPY, QDPY},
		{QDPHP, QDPHP},
		{QDCLC, QDCLC},
		{"", ""},
	} {
		t.Run(
			tc.productCode, func(t *testing.T) {
				assert.Equal(t, tc.toolName, CloudBaselineToolName(tc.productCode))
			},
		)
	}
}
