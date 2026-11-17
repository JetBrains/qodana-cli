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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	fus "github.com/JetBrains/fus-reporting-api-go"
	"github.com/JetBrains/qodana-cli/internal/platform/version"
)

// The PostHog project of Qodana Cloud, where CLI events join its organization and project groups. The project key
// is public by design: it can only send events.
var (
	postHogHost = "https://eu.i.posthog.com"
	postHogKey  = "phc_UlVZwwfIjUfwYT6KKLUyNesxu8Ro7BgwSKPGZLTqf2v"
)

type postHogEvent struct {
	Event      string         `json:"event"`
	DistinctId string         `json:"distinct_id"`
	Timestamp  time.Time      `json:"timestamp"`
	Properties map[string]any `json:"properties"`
}

// sendToPostHog sends the events validated for FUS, named group.event as Qodana Cloud names them. The subject is the
// device, stable per repository, without a person profile; Qodana Cloud's organizations and projects are groups.
func sendToPostHog(ctx context.Context, logEvents []fus.LogEvent) error {
	batch := make([]postHogEvent, 0, len(logEvents))
	for _, e := range logEvents {
		device := e.IDs["device"]
		// cli_version: events have a version field of their own, of the linter or the engine
		properties := map[string]any{"session": e.Session, "cli_version": e.Build, "$process_person_profile": false}
		maps.Copy(properties, e.Event.Data)
		groups := map[string]any{}
		if isRepositoryDevice(device) {
			groups["repository"] = device
		}
		for group, key := range map[string]string{"organization": "org_id", "project": "project_id"} {
			// a value the validator rejected, like validation.unmatched_rule, is no id
			if id, ok := properties[key].(string); ok && !strings.HasPrefix(id, "validation.") {
				groups[group] = id
			} else {
				delete(properties, key)
			}
		}
		if len(groups) > 0 {
			properties["$groups"] = groups
		}
		batch = append(batch, postHogEvent{
			Event:      e.Group.ID + "." + e.Event.ID,
			DistinctId: device,
			Timestamp:  time.UnixMilli(e.Time).UTC(),
			Properties: properties,
		})
	}
	body, err := json.Marshal(map[string]any{"api_key": postHogKey, "batch": batch})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, postHogHost+"/batch/", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", productCode+"/"+version.Version)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	_ = response.Body.Close()
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("PostHog answered %s", response.Status)
	}
	return nil
}
