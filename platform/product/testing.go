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
	"fmt"
	"os"
	"testing"
)

// RequireNightlyAuth skips the test if running against nightly (unreleased) versions
// and the required QD_PRODUCT_INTERNAL_AUTH environment variable is not set.
// It also sets up QD_PRODUCT_INTERNAL_FEED for nightly builds.
func RequireNightlyAuth(t *testing.T) {
	//goland:noinspection GoBoolExpressions
	if !IsReleased {
		_, exists := os.LookupEnv("QD_PRODUCT_INTERNAL_AUTH")
		if !exists {
			t.Skipf("Requires TC auth for downloading nightly versions")
		}
		t.Setenv(
			"QD_PRODUCT_INTERNAL_FEED",
			fmt.Sprintf("https://packages.jetbrains.team/files/p/sa/qdist/%s/feed.json", ReleaseVersion),
		)
	}
}
