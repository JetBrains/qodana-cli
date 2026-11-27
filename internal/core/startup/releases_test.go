/*
 * Copyright 2021-2026 JetBrains s.r.o.
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
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/version"
)

func TestSelectLatestCompatibleRelease(t *testing.T) {
	release := func(date, releaseType, major, printable string) ReleaseInfo {
		return ReleaseInfo{Date: date, Type: releaseType, MajorVersion: &major, PrintableReleaseType: &printable}
	}
	prod := &Product{
		Releases: []ReleaseInfo{
			release("2026-11-01", "eap", product.ReleaseVersion, "EAP"),
			release("2026-11-02", "release", product.ReleaseVersion, "release"),
			release("2026-11-20", "eap", "2027.1", "nightly"),
			release("2026-11-26", "eap", "2027.1", "nightly"),
			release("2026-11-27", "eap", "2027.2", "nightly"),
		},
	}

	tests := []struct {
		name     string
		version  string
		reqType  string
		wantDate string
	}{
		{"nightly CLI takes the newest nightly of its major version", "2027.1-nightly", "eap", "2026-11-26"},
		{"nightly CLI takes it whatever the requested type", "2027.1-nightly", "release", "2026-11-26"},
		{"regular CLI takes the release of its version", "2026.3.1", "release", "2026-11-02"},
		{"regular CLI takes the EAP of its version", "2026.3.1", "eap", "2026-11-01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := version.Version
			version.Version = tt.version
			t.Cleanup(func() { version.Version = previous })

			got := selectLatestCompatibleRelease(prod, tt.reqType)
			if got == nil || got.Date != tt.wantDate {
				t.Fatalf("selectLatestCompatibleRelease(%q) = %+v, want the release of %s", tt.reqType, got, tt.wantDate)
			}
		})
	}
}
