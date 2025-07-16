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

package startup

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2025/platform/product"
	"testing"
)

func TestGetProductByCode(t *testing.T) {
	t.Setenv(
		"QD_PRODUCT_INTERNAL_FEED",
		fmt.Sprintf("https://packages.jetbrains.team/files/p/sa/qdist/%s/feed.json", product.ReleaseVersion),
	)
	prod, err := GetProductByCode("RD")
	if err != nil {
		t.Fatalf("Error getting prod: %s", err)
	}
	if prod == nil {
		t.Fatalf("Product is nil")
	}

	eap := SelectLatestCompatibleRelease(prod, "eap")
	if eap == nil {
		t.Fatalf("EAP is nil")
	}

	if product.IsReleased {
		release := SelectLatestCompatibleRelease(prod, "release")
		if release == nil {
			t.Fatalf("Release is nil")
		}
	}
}
