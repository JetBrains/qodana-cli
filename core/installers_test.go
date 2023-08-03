/*
* Copyright 2021-2023 JetBrains s.r.o.
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

package core

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGetIde(t *testing.T) {
	installers := []string{QDJVMC, QDJVM, QDPHP, QDPY, QDPYC, QDJS, QDGO, QDNET}
	for _, installer := range installers {
		ide := getIde(installer)
		if ide == nil {
			if installer != QDPHP {
				t.Fail()
			}
		} else if installer == QDPHP {
			// release happened, fix the test
			t.Fail()
		}
		eap := getIde(installer + "-EAP")
		if eap == nil {
			t.Fail()
		}
	}
}

func TestDownloadAndInstallIDE(t *testing.T) {
	ides := []string{QDPY, "QDNET-EAP"} // QDPY requires exe on Windows, QDNET - does not
	for _, ide := range ides {
		DownloadAndInstallIDE(ide, t)
	}
}

func DownloadAndInstallIDE(ideName string, t *testing.T) {
	tempDir, err := os.MkdirTemp("", "productByCode")
	if err != nil {
		ErrorMessage("Cannot create temp dir: %s", err)
	}

	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			ErrorMessage("Cannot clean up temp dir: %s", err)
		}
	}(tempDir) // clean up

	ide := downloadAndInstallIDE(ideName, tempDir)
	if ide == "" {
		ErrorMessage("Cannot install %s", ideName)
		t.Fail()
	}

	if //goland:noinspection GoBoolExpressions
	runtime.GOOS == "darwin" {
		ide = filepath.Join(ide, "Contents")
	}
	productInfo := readIdeProductInfo(ide)
	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			ErrorMessage("Cannot clean up temp dir: %s", err)
		}
	}(ide) // clean up
	if productInfo == nil {
		t.Fail()
	}
}
