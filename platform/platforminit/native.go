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

package platforminit

// currently contains only the logic for .NET products
import (
	"bufio"
	"github.com/JetBrains/qodana-cli/v2024/platform/product"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func IsNativeRequired(projectDir string, ide string) bool {
	if ide != product.QDNET {
		return false
	}
	return containsUnityProjects(projectDir) || containsDotNetFxProjects(projectDir)
}

func containsUnityProjects(projectDir string) bool {
	assetsPath := filepath.Join(projectDir, "Assets")
	projectSettingsPath := filepath.Join(projectDir, "ProjectSettings")

	if !isDirectory(assetsPath) {
		return false
	}
	if !isDirectory(projectSettingsPath) {
		return false
	}

	isUnityProject := false

	err := filepath.Walk(
		projectSettingsPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.Name() == "ProjectVersion.txt" || strings.ToLower(filepath.Ext(path)) == ".asset" {
				isUnityProject = true
			}

			return nil
		},
	)
	if err != nil {
		return isUnityProject
	}

	return isUnityProject
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func containsDotNetFxProjects(projectDir string) bool {
	var found = false
	var waitGroup sync.WaitGroup
	var mutex = &sync.Mutex{}

	err := filepath.Walk(
		projectDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			isFound := false
			mutex.Lock()
			isFound = found
			mutex.Unlock()

			if info.IsDir() || filepath.Ext(path) != ".csproj" || isFound {
				return nil
			}

			waitGroup.Add(1)

			go func(path string) {
				defer waitGroup.Done()

				match, err := checkFile(path)
				if err != nil {
					return
				}

				if match {
					mutex.Lock()
					found = true
					mutex.Unlock()
				}
			}(path)

			return nil
		},
	)
	if err != nil {
		return false
	}

	waitGroup.Wait()

	return found
}

func checkFile(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Error(err)
		}
	}(file)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "<TargetFramework") && (strings.Contains(line, "net4") || strings.Contains(
			line,
			"v4",
		)) {
			return true, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return false, err
	}

	return false, nil
}
