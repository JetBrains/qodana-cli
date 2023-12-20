package platform

import (
	"encoding/json"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2023/tooling"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

var wg sync.WaitGroup

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

func sendFuserEvents(ch chan tooling.FuserEvent, events *[]tooling.FuserEvent, opts *QodanaOptions, deviceId string) {
	wg.Wait()
	close(ch)
	if opts.NoStatistics {
		println("Statistics disabled, skipping FUS")
		return
	}
	linterOptions := opts.GetLinterSpecificOptions()
	if linterOptions == nil {
		log.Error(fmt.Errorf("linter specific options are not set"))
		return
	}
	linterInfo := (*linterOptions).GetInfo(opts)
	mountInfo := (*linterOptions).GetMountInfo()

	fatBytes, err := json.Marshal(*events)
	if err != nil {
		log.Error(fmt.Errorf("failed to marshal events to json: %w", err))
		return
	}

	// create a file in temp dir
	fileName := filepath.Join(opts.GetTmpResultsDir(), "fuser.json")
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

	args := []string{QuoteForWindows(mountInfo.JavaPath), "-jar", QuoteForWindows(mountInfo.Fuser), deviceId, linterInfo.ProductCode, linterInfo.LinterVersion, QuoteForWindows(fileName)}
	if os.Getenv("GO_TESTING") == "true" {
		args = append(args, "true")
	}
	_, _ = LaunchAndLog(opts, "fuser", args...)
}

func currentTimestamp() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func logProjectOpen(ch chan tooling.FuserEvent) {
	wg.Add(1)
	// get current time in milliseconds
	ch <- tooling.FuserEvent{
		GroupId:   "qd.cl.lifecycle",
		EventName: "project.opened",
		EventData: map[string]string{},
		Time:      currentTimestamp(),
		State:     false,
	}
}

func logProjectClose(ch chan tooling.FuserEvent) {
	wg.Add(1)
	ch <- tooling.FuserEvent{
		GroupId:   "qd.cl.lifecycle",
		EventName: "project.closed",
		EventData: map[string]string{},
		Time:      currentTimestamp(),
		State:     false,
	}
}

func logOs(ch chan tooling.FuserEvent) {
	wg.Add(1)
	ch <- tooling.FuserEvent{
		GroupId:   "qd.cl.system.os",
		EventName: "os.name",
		EventData: map[string]string{"name": runtime.GOOS, "arch": runtime.GOARCH},
		Time:      currentTimestamp(),
		State:     true,
	}
}
