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

package platform

import (
	"os"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
)

func TestCheckEAP_NotEAP(t *testing.T) {
	// When isEap is false, function should return early without doing anything
	CheckEAP("2024-01-01T00:00:00Z", false)
	// If we get here without panicking, test passes
}

func TestCheckEAP_ValidEAP(t *testing.T) {
	// Set up a future build date so EAP is still valid
	futureDate := time.Now().Add(-10 * 24 * time.Hour).Format(time.RFC3339)

	// Capture log.Fatal calls
	var fatalCalled bool
	log.StandardLogger().ExitFunc = func(int) { fatalCalled = true }
	defer func() { log.StandardLogger().ExitFunc = os.Exit }()

	CheckEAP(futureDate, true)

	if fatalCalled {
		t.Error("Expected CheckEAP not to call Fatal for valid EAP")
	}
}

func TestCheckEAP_ExpiredEAP(t *testing.T) {
	// Set up a past build date so EAP is expired (more than 60 days ago)
	expiredDate := time.Now().Add(-70 * 24 * time.Hour).Format(time.RFC3339)

	// Capture log.Fatal calls
	var fatalCalled bool
	log.StandardLogger().ExitFunc = func(int) { fatalCalled = true }
	defer func() { log.StandardLogger().ExitFunc = os.Exit }()

	CheckEAP(expiredDate, true)

	if !fatalCalled {
		t.Error("Expected CheckEAP to call Fatal for expired EAP")
	}
}

func TestCheckEAP_InvalidDateFormat(t *testing.T) {
	// Capture log.Fatal calls
	var fatalCalled bool
	log.StandardLogger().ExitFunc = func(int) { fatalCalled = true }
	defer func() { log.StandardLogger().ExitFunc = os.Exit }()

	CheckEAP("invalid-date", true)

	if !fatalCalled {
		t.Error("Expected CheckEAP to call Fatal for invalid date format")
	}
}
