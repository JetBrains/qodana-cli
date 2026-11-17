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

package telemetry

import (
	"cmp"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// A linter container started by the CLI does not send statistics: its CLI hands what only it knows (the license,
// a missing start commit of the diff it runs and the precise finish reason) over to the starting CLI through a file in the mounted cache dir, so a run is
// reported once, in one session.
const handoverEnv = "QODANA_STATISTICS_FILE"

// handoverName is random: a file left in a kept cache dir by a crashed run is never taken for the file of this one.
var handoverName = "qodana-statistics-" + uuid.NewString() + ".json"

// handoverVersion changes with the format of the handover file: the CLI of a linter image is of another release.
const handoverVersion = 1

type handover struct {
	Version   int     `json:"version"`
	Events    []event `json:"events"`
	ProjectId string  `json:"projectId,omitempty"`
	OrgId     string  `json:"orgId,omitempty"`
	// StartCommitMissing tells that the diff the host left to the container ran on the current state instead
	StartCommitMissing bool   `json:"startCommitMissing,omitempty"`
	Reason             string `json:"reason"`
	Allowed            bool   `json:"allowed"`
}

// AllowContainer forbids sending the run of a linter container which gets a license-only token, also one given
// with --env, which this CLI does not check: the container sends nothing then, nor hands anything over.
func AllowContainer(env []string) {
	Allow(!isLicenseOnly(env))
}

// ContainerEnv returns the environment variable for a linter container started by this run.
func ContainerEnv(containerCacheDir string) string {
	mu.Lock()
	defer mu.Unlock()
	if !enabled {
		return doNotTrackEnv + "=1"
	}
	return handoverEnv + "=" + path.Join(containerCacheDir, handoverName)
}

// CollectFromContainer adds what the CLI in the finished linter container handed over to this run. A run starts
// a container for each analyzed commit, their license events are the same.
func CollectFromContainer(cacheDir string) {
	file := filepath.Join(cacheDir, handoverName)
	content, err := os.ReadFile(file)
	if err != nil {
		return
	}
	_ = os.Remove(file)
	var h handover
	if err := json.Unmarshal(content, &h); err != nil {
		log.Debugf("Failed to read statistics of the linter container: %v", err)
		return
	}
	if h.Version != handoverVersion {
		log.Debugf("Statistics of the linter container are of version %d, not %d", h.Version, handoverVersion)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if !enabled {
		return
	}
	for _, e := range h.Events {
		if !slices.ContainsFunc(events, func(tracked event) bool { return tracked.Id == e.Id }) {
			events = append(events, e)
		}
		// the run stops at the license, as a native one does, whatever this run does after the container; an
		// expired license is told by the exit code of the analysis, as a native one is
		if e.Id == licenseRejected && e.Data["reject_reason"] != "expired" {
			failedStage = licenseRejected
		}
	}
	projectId, orgId = cmp.Or(h.ProjectId, projectId), cmp.Or(h.OrgId, orgId)
	if h.StartCommitMissing {
		scenario = ScenarioStartCommitMissing
	}
	// the reason of the last container: a failure is more precise than its exit code, a success is no failure
	knownReason = ""
	if !isSuccess(h.Reason) {
		knownReason = h.Reason
	}
	allowed = allowed && h.Allowed
}

func handoverFile() string {
	return os.Getenv(handoverEnv)
}

func handOver(file string, h handover) {
	h.Version = handoverVersion
	var licenseEvents []event
	for _, e := range h.Events {
		if e.Id == licenseObtained || e.Id == licenseRejected {
			licenseEvents = append(licenseEvents, e)
		}
	}
	h.Events = licenseEvents
	content, err := json.Marshal(h)
	if err == nil {
		err = os.WriteFile(file, content, 0o644)
	}
	if err != nil {
		log.Debugf("Failed to hand the statistics over: %v", err)
	}
}
