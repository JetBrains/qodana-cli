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
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	fus "github.com/JetBrains/fus-reporting-api-go"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

const (
	sendTimeout = 2 * time.Second
	// fusTestEnv sends the events of any build to the FUS test config, to check them with the FUS tools, not to PostHog
	fusTestEnv = "QODANA_FUS_TEST"
)

// sender sends events with the time they happened, unlike fus.Logger, which stamps the tracking time. Values are
// not escaped: the scheme accepts only escape-safe ones. Validated events go to FUS and to PostHog.
type sender struct {
	client    *fus.Client
	postHog   bool
	validator *fus.Validator
	base      fus.LogEvent
}

// newSender returns nil when statistics cannot be sent anywhere. device is the device id the CLI gives the linters,
// build the version of the binary.
func newSender(device string, build string) *sender {
	s, err := createSender(device, build)
	if err != nil {
		log.Debugf("Statistics are disabled: %v", err)
	}
	return s
}

// releaseVersion is the version of a release, like 2026.2 or 2026.2.3: development, snapshot and nightly builds
// are of internal use, which is not reported.
var releaseVersion = regexp.MustCompile(`^\d{4}\.\d+(\.\d+)?$`)

func createSender(device string, build string) (*sender, error) {
	fusTest := isTruthy(os.Getenv(fusTestEnv))
	if !fusTest && !releaseVersion.MatchString(build) {
		return nil, fmt.Errorf("%s is not a release", build)
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	dataDir := filepath.Join(cacheDir, "JetBrains", "Qodana", "fus")
	// PostHog follows FUS: a product without a FUS config sends nowhere. A cached config is used whatever its age
	// when the fetch fails, so removing the config stops only the machines without it.
	var config *fus.FUSConfig
	if fusTest {
		config, err = fus.FetchTestConfig(recorderID, productCode)
	} else {
		config, err = fus.LoadOrFetchConfig(recorderID, productCode, build, dataDir, fus.RegionAll)
	}
	if err != nil {
		return nil, err
	}
	deviceIds, bucket, err := deviceIdentity(device, dataDir)
	if err != nil {
		return nil, err
	}
	scheme, err := definition.BuildValidationScheme()
	if err != nil {
		return nil, err
	}
	validator, err := fus.NewValidator(scheme)
	if err != nil {
		return nil, err
	}
	return &sender{
		client:    fus.NewClient(config.SendEndpoint, sendTimeout, productCode+"/"+build),
		postHog:   !fusTest,
		validator: validator,
		base: fus.LogEvent{
			Recorder: fus.Recorder{ID: recorderID, Version: recorderVersion},
			Product:  productCode,
			IDs:      deviceIds,
			Build:    build,
			Session:  uuid.NewString(),
			Bucket:   bucket,
		},
	}, nil
}

// deviceIdentity returns the device ids and the bucket of the run: the ones of the repository device as the linter
// sends them, or of the machine when the project has no repository device.
func deviceIdentity(device string, dataDir string) (map[string]string, int, error) {
	if isRepositoryDevice(device) {
		return map[string]string{"device": device}, ideBucket(device), nil
	}
	machine, err := fus.GetOrCreateDeviceID(dataDir)
	if err != nil {
		return nil, 0, err
	}
	return map[string]string{"device": fus.Anonymize([]byte(salt), machine)}, fus.ComputeBucket(machine), nil
}

var repositoryDeviceRegexp = regexp.MustCompile(`^` + repositoryDevice + `$`)

func isRepositoryDevice(device string) bool {
	return repositoryDeviceRegexp.MatchString(device) && device != noRepositoryDevice
}

// ideBucket is the bucket the IDE computes for a device id, MathUtil.nonNegativeAbs(deviceId.hashCode()) % 256: the
// CLI and its linter fall into the same bucket.
func ideBucket(device string) int {
	var hash int32
	for _, char := range device {
		hash = 31*hash + char
	}
	if hash == math.MinInt32 {
		hash = math.MaxInt32
	}
	if hash < 0 {
		hash = -hash
	}
	return int(hash % 256)
}

func (s *sender) send(events []event) error {
	logEvents := make([]fus.LogEvent, 0, len(events))
	for _, e := range events {
		logEvent := s.base
		logEvent.Time = e.Time
		logEvent.Group = fus.Group(e.Group, groupVersion)
		logEvent.Event = fus.EventAction{ID: e.Id, Data: e.Data, Count: 1}
		logEvent, dropped := s.validator.Validate(logEvent)
		if dropped || len(logEvent.Event.Data) > fus.MaxDataFields {
			continue
		}
		logEvents = append(logEvents, logEvent)
	}
	if len(logEvents) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	var fusErr, postHogErr error
	var wg sync.WaitGroup
	wg.Go(func() { _, fusErr = s.client.SendBatched(ctx, logEvents) })
	if s.postHog {
		wg.Go(func() { postHogErr = sendToPostHog(ctx, s.base.Build, logEvents) })
	}
	wg.Wait()
	return errors.Join(fusErr, postHogErr)
}
