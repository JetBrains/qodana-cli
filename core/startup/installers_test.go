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
	"github.com/JetBrains/qodana-cli/v2025/platform/msg"
	"github.com/JetBrains/qodana-cli/v2025/platform/product"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGetIde(t *testing.T) {
	//os.Setenv("QD_PRODUCT_INTERNAL_FEED", "https://data.services.jetbrains.com/products")
	for _, linter := range product.AllNativeLinters {
		ide := getIde(linter.NativeAnalyzer())
		if ide == nil {
			t.Fail()
		}
		if runtime.GOOS != "darwin" {
			eap := getIde(&product.NativeAnalyzer{Linter: linter, Eap: true})
			if eap == nil {
				t.Fail()
			}
		}
	}
}

func TestDownloadAndInstallIDE(t *testing.T) {
	linters := []product.Linter{product.GoLinter}
	for _, linter := range linters {
		DownloadAndInstallIDE(linter, t)
	}
}

func DownloadAndInstallIDE(linter product.Linter, t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	tempDir := filepath.Join(homeDir, ".qodana_scan_", "ideTest")
	err = os.RemoveAll(tempDir)
	if err != nil {
		msg.ErrorMessage("Cannot remove previous temp dir: %s", err)
		t.Fail()
	}

	err = os.MkdirAll(tempDir, 0755)
	if err != nil {
		msg.ErrorMessage("Cannot create temp dir: %s", err)
		t.Fail()
	}

	analyzer := linter.NativeAnalyzer()
	ide := downloadAndInstallIDE(analyzer, tempDir, nil)

	if ide == "" {
		msg.ErrorMessage("Cannot install %s", linter.Name)
		t.Fail()
	}
	prodInfo, err := product.ReadIdeProductInfo(ide)
	if err != nil || prodInfo == nil {
		t.Fatalf("Failed to read IDE product info: %v", err)
	}
	prod := product.GuessProduct(ide, analyzer)

	prepareCustomPlugins(prod)
	disabledPluginsFilePath := prod.DisabledPluginsFilePath()
	if _, err := os.Stat(disabledPluginsFilePath); err != nil {
		t.Fatalf("Cannot find disabled plugins file: %s", disabledPluginsFilePath)
	}

	customPluginsFilePath := prod.CustomPluginsPath()
	if _, err := os.Stat(customPluginsFilePath); err != nil {
		t.Fatalf("Cannot find custom plugins folder: %s", customPluginsFilePath)
	}
}
