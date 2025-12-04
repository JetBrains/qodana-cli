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

package git

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/sirupsen/logrus"
)

var LOGGER = NewLoggerManager()

// LoggerManager manages loggers for different commands
type LoggerManager struct {
	loggers map[string]*logrus.Logger
	mu      sync.Mutex
}

// NewLoggerManager creates a new LoggerManager
func NewLoggerManager() *LoggerManager {
	return &LoggerManager{
		loggers: make(map[string]*logrus.Logger),
	}
}

// GetLogger returns a logger for the given command
func (lm *LoggerManager) GetLogger(logdir string, command string) (*logrus.Logger, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if logger, exists := lm.loggers[command]; exists {
		return logger, nil
	}

	logger := logrus.New()

	if _, err := os.Stat(logdir); os.IsNotExist(err) {
		// When docker being launched, directory does not get created, returning nil
		return nil, nil
	}
	logFileName := filepath.Join(logdir, filepath.Base(command)+".log")

	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}

	logger.SetOutput(logFile)

	logger.SetFormatter(
		&logrus.TextFormatter{
			FullTimestamp: true,
		},
	)

	lm.loggers[command] = logger

	return logger, nil
}
