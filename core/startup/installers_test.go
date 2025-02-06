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

package startup

import (
	"github.com/JetBrains/qodana-cli/v2024/platform/msg"
	product2 "github.com/JetBrains/qodana-cli/v2024/platform/product"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGetIde(t *testing.T) {
	//os.Setenv("QD_PRODUCT_INTERNAL_FEED", "https://data.services.jetbrains.com/products")
	for _, installer := range product2.AllNativeCodes {
		ide := getIde(installer)
		if ide == nil {
			t.Fail()
		}
		if runtime.GOOS != "darwin" {
			eap := getIde(installer + "-EAP")
			if eap == nil {
				t.Fail()
			}
		}
	}
}

func TestDownloadAndInstallIDE(t *testing.T) {
	ides := []string{"QDGO"}
	for _, ide := range ides {
		DownloadAndInstallIDE(ide, t)
	}
}

func DownloadAndInstallIDE(ideName string, t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	tempDir := filepath.Join(homeDir, ".qodana_scan_", "ideTest")
	err = os.MkdirAll(tempDir, 0755)
	if err != nil {
		msg.ErrorMessage("Cannot create temp dir: %s", err)
		t.Fail()
	}

	ide := downloadAndInstallIDE(ideName, "", tempDir, nil)

	if ide == "" {
		msg.ErrorMessage("Cannot install %s", ideName)
		t.Fail()
	}
	prod, err := product2.ReadIdeProductInfo(ide)
	if err != nil || prod == nil {
		t.Fatalf("Failed to read IDE product info: %v", err)
	}
	if prod.ProductCode == "" {
		t.Fail()
	}
}
