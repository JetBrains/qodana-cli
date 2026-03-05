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

package tooling

import (
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/JetBrains/qodana-cli/internal/platform/product"
)

//go:generate go run scripts/download-libs.go
//go:embed libs/*.jar
var libs embed.FS

type Library string

const (
	BaselineCli     Library = "libs/baseline-cli*.jar"
	ConfigLoaderCli Library = "libs/config-loader-cli*.jar"
	Fuser           Library = "libs/qodana-fuser*.jar"
	PublisherCli    Library = "libs/publisher-cli*.jar"
	ReportConverter Library = "libs/intellij-report-converter*.jar"
	QodanaWebUi     Library = "libs/qodana-web-ui*.jar"
)

func (library Library) GetLibPath(cacheDir string) string {
	matchedFile := findLibFile(library)
	libPath := extractLib(cacheDir, matchedFile)
	return libPath
}

func findLibFile(library Library) string {
	libPattern := string(library)
	matches, err := fs.Glob(libs, libPattern)
	if err != nil {
		log.Fatalf("Failed to glob for %s jar: %s", libPattern, err)
	}
	if len(matches) != 1 {
		log.Fatalf("expected exactly 1 embedded %s jar, got %d: %v", string(library), len(matches), matches)
	}
	return matches[0]
}

func extractLib(cacheDir string, matchedFile string) string {
	libFileName := filepath.Base(matchedFile)
	libPath := filepath.Join(GetToolsMountPath(cacheDir), libFileName)
	if _, err := os.Stat(libPath); err != nil {
		if os.IsNotExist(err) {
			jarFileBytes, err := libs.ReadFile(matchedFile)
			if err != nil {
				log.Fatalf("Failed to read %s library: %s", libFileName, err)
			}
			err = os.WriteFile(libPath, jarFileBytes, 0644)
			if err != nil {
				log.Fatalf("Failed to write %s : %s", libFileName, err)
			}
		}
	}
	return libPath
}

func GetToolsMountPath(cacheDir string) string {
	mountPath := filepath.Join(cacheDir, product.ShortVersion)
	if _, err := os.Stat(mountPath); err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(mountPath, 0755)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
	return mountPath
}
