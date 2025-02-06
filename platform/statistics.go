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
	"encoding/json"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/cloud"
	"github.com/JetBrains/qodana-cli/v2024/platform/thirdpartyscan"
	"github.com/JetBrains/qodana-cli/v2024/platform/utils"
	"github.com/JetBrains/qodana-cli/v2024/tooling"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

var wg sync.WaitGroup

const qodanaProjectId = "system_qdcld_project_id"

func createFuserEventChannel(events *[]tooling.FuserEvent) chan tooling.FuserEvent {
	ch := make(chan tooling.FuserEvent)
	guid := uuid.New().String()
	go func() {
		for event := range ch {
			event.SessionId = guid
			*events = append(*events, event)
			wg.Done()
		}
	}()
	return ch
}

func sendFuserEvents(
	ch chan tooling.FuserEvent,
	events *[]tooling.FuserEvent,
	c thirdpartyscan.Context,
	deviceId string,
) {
	linterInfo := c.LinterInfo()
	mountInfo := c.MountInfo()
	wg.Wait()
	close(ch)
	if c.NoStatistics() {
		println("Statistics disabled, skipping FUS")
		return
	}
	if !cloud.Token.IsAllowedToSendFUS() {
		println("You are not allowed to send FUS")
		return
	}

	fatBytes, err := json.Marshal(*events)
	if err != nil {
		log.Error(fmt.Errorf("failed to marshal events to json: %w", err))
		return
	}

	// create a file in temp dir
	fileName := filepath.Join(GetTmpResultsDir(c.ResultsDir()), "fuser.json")
	f, err := os.Create(fileName)
	if err != nil {
		log.Error(fmt.Errorf("failed to create file %s: %w", fileName, err))
		return
	}

	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			log.Error(fmt.Errorf("error closing resulting FUS file: %w", err))
		}
	}(f)

	_, err = f.Write(fatBytes)
	if err != nil {
		log.Error(fmt.Errorf("failed to write events to file %s: %w", fileName, err))
		return
	}

	args := []string{
		utils.QuoteForWindows(mountInfo.JavaPath),
		"-jar",
		utils.QuoteForWindows(mountInfo.Fuser),
		deviceId,
		linterInfo.ProductCode,
		linterInfo.LinterVersion,
		utils.QuoteForWindows(fileName),
	}
	if os.Getenv("GO_TESTING") == "true" {
		args = append(args, "true")
	}
	_, _, _, _ = utils.LaunchAndLog(c.LogDir(), "fuser", args...)
}

func currentTimestamp() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func commonEventData(linterInfo thirdpartyscan.LinterInfo, projectIdHash string) map[string]string {
	eventData := map[string]string{"version": linterInfo.GetMajorVersion()}
	if projectIdHash != "" {
		eventData[qodanaProjectId] = projectIdHash
	}
	return eventData
}

func logProjectOpen(ch chan tooling.FuserEvent, linterInfo thirdpartyscan.LinterInfo, projectIdHash string) {
	wg.Add(1)
	eventData := commonEventData(linterInfo, projectIdHash)
	ch <- tooling.FuserEvent{
		GroupId:   "qd.cl.lifecycle",
		EventName: "project.opened",
		EventData: eventData,
		Time:      currentTimestamp(),
		State:     false,
	}
}

func logProjectClose(ch chan tooling.FuserEvent, linterInfo thirdpartyscan.LinterInfo, projectIdHash string) {
	wg.Add(1)
	eventData := commonEventData(linterInfo, projectIdHash)
	ch <- tooling.FuserEvent{
		GroupId:   "qd.cl.lifecycle",
		EventName: "project.closed",
		EventData: eventData,
		Time:      currentTimestamp(),
		State:     false,
	}
}

func logOs(ch chan tooling.FuserEvent, linterInfo thirdpartyscan.LinterInfo, projectIdHash string) {
	wg.Add(1)
	eventData := commonEventData(linterInfo, projectIdHash)
	eventData["name"] = runtime.GOOS
	eventData["arch"] = runtime.GOARCH
	ch <- tooling.FuserEvent{
		GroupId:   "qd.cl.system.os",
		EventName: "os.name",
		EventData: eventData,
		Time:      currentTimestamp(),
		State:     true,
	}
}
