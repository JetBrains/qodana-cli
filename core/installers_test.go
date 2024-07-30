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

package core

import (
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGetIde(t *testing.T) {
	err := os.Setenv("QD_PRODUCT_INTERNAL_FEED", "https://data.services.jetbrains.com/products")
	if err != nil {
		t.Fatal(err)
	}
	for _, installer := range platform.AllNativeCodes {
		//ide := getIde(installer)
		//if ide == nil {
		//	t.Fail()
		//}
		eap := getIde(installer + "-EAP")
		if eap == nil {
			t.Fail()
		}
	}
}

func TestDownloadAndInstallIDE(t *testing.T) {
	err := os.Setenv("QD_PRODUCT_INTERNAL_FEED", "https://data.services.jetbrains.com/products")
	if err != nil {
		t.Fatal(err)
	}
	ides := []string{"QDGO-EAP"}
	for _, ide := range ides {
		DownloadAndInstallIDE(ide, t)
	}
}

func DownloadAndInstallIDE(ideName string, t *testing.T) {
	tempDir, err := os.MkdirTemp("", "productByCode")
	if err != nil {
		platform.ErrorMessage("Cannot create temp dir: %s", err)
	}

	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			platform.ErrorMessage("Cannot clean up temp dir: %s", err)
		}
	}(tempDir) // clean up

	opts := &QodanaOptions{
		&platform.QodanaOptions{
			Ide: ideName,
		},
	}
	ide := downloadAndInstallIDE(opts, tempDir, nil)
	if ide == "" {
		platform.ErrorMessage("Cannot install %s", ideName)
		t.Fail()
	}

	if //goland:noinspection GoBoolExpressions
	runtime.GOOS == "darwin" {
		ide = filepath.Join(ide, "Contents")
	}
	prod, err := readIdeProductInfo(ide)
	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			platform.ErrorMessage("Cannot clean up temp dir: %s", err)
		}
	}(ide) // clean up
	if prod.ProductCode == "" {
		t.Fail()
	}
}
