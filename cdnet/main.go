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

package cdnet

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"os/signal"
	"syscall"
)

const (
	productCode = "QDNETC"
	linterName  = "Qodana Community for .NET"
)

var InterruptChannel chan os.Signal
var version = "2023.3"
var buildDateStr = "2023-12-05T10:52:23Z"

//var isEap = true

func main() {
	InterruptChannel = make(chan os.Signal, 1)
	signal.Notify(InterruptChannel, os.Interrupt)
	signal.Notify(InterruptChannel, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-InterruptChannel
		fmt.Println("Interrupting Qodana...")
		log.SetOutput(io.Discard)
		os.Exit(0)
	}()
	Execute(productCode, linterName, version, buildDateStr, true)
}
