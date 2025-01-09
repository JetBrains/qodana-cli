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
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-enry/go-enry/v2"

	log "github.com/sirupsen/logrus"
)

const (
	QodanaSarifName = "qodana.sarif.json"
	configName      = "qodana"
	ReleaseVersion  = "2024.3"
	shortVersion    = "243"
	isReleased      = true
)

// langsProductCodes is a map of languages to linters.
var langsProductCodes = map[string][]string{
	"Java":              {QDJVM, QDJVMC, QDAND, QDANDC},
	"Kotlin":            {QDJVM, QDJVMC, QDAND, QDANDC},
	"PHP":               {QDPHP},
	"Python":            {QDPY, QDPYC},
	"JavaScript":        {QDJS},
	"TypeScript":        {QDJS},
	"Go":                {QDGO},
	"C#":                {QDNET, QDNETC},
	"F#":                {QDNET},
	"Visual Basic .NET": {QDNET, QDNETC},
	"C":                 {QDCL, QDNET},
	"C++":               {QDCL, QDNET},
}

var AllSupportedPaidCodes = []string{QDJVM, QDPHP, QDPY, QDJS, QDGO, QDNET, QDAND}
var AllSupportedFreeCodes = []string{QDJVMC, QDPYC, QDANDC, QDNETC, QDCL}

func allImages(codes []string) []string {
	var images []string
	for _, code := range codes {
		images = append(images, Image(code))
	}
	return images
}

var AllSupportedFreeImages = allImages(AllSupportedFreeCodes)

// AllImages is a list of all supported linters.
var AllImages = append(allImages(AllSupportedPaidCodes), AllSupportedFreeImages...)

// AllCodes is a list of codes for all supported linters.
var AllCodes = append(AllSupportedPaidCodes, AllSupportedFreeCodes...)

// ignoredDirectories is a list of directories that should be ignored by the configurator.
var ignoredDirectories = []string{
	".idea",
	".vscode",
	".git",
}

// isInIgnoredDirectory returns true if the given path should be ignored by the configurator.
func isInIgnoredDirectory(path string) bool {
	parts := strings.Split(path, string(os.PathSeparator))
	for _, part := range parts {
		for _, ignored := range ignoredDirectories {
			if part == ignored {
				return true
			}
		}
	}
	return false
}

// recognizeDirLanguages returns the languages detected in the given directory.
func recognizeDirLanguages(projectPath string) ([]string, error) {
	const limitKb = 64
	out := make(map[string]int)
	err := filepath.Walk(projectPath, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return filepath.SkipDir
		}

		if f.Mode().IsDir() && !f.Mode().IsRegular() {
			return nil
		}

		relpath, err := filepath.Rel(projectPath, path)
		if err != nil {
			return nil
		}

		if relpath == "." {
			return nil
		}

		if f.IsDir() {
			relpath = relpath + string(os.PathSeparator)
		}
		if isInIgnoredDirectory(path) || enry.IsVendor(relpath) || enry.IsDotFile(relpath) ||
			enry.IsDocumentation(relpath) || enry.IsConfiguration(relpath) ||
			enry.IsGenerated(relpath, nil) {
			if f.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if f.IsDir() {
			return nil
		}

		content, err := readFile(path, limitKb)
		if err != nil {
			return nil
		}

		if enry.IsGenerated(relpath, content) {
			return nil
		}

		language := enry.GetLanguage(filepath.Base(path), content)
		if language == enry.OtherLanguage {
			return nil
		}

		if enry.GetLanguageType(language) != enry.Programming {
			return nil
		}

		out[language] += 1
		return nil
	})
	if err != nil {
		return nil, err
	}
	type languageCount struct {
		Language string
		Count    int
	}
	langCounts := make([]languageCount, 0, len(out))
	for language, count := range out {
		langCounts = append(langCounts, languageCount{Language: language, Count: count})
	}
	sort.Slice(langCounts, func(i, j int) bool {
		return langCounts[i].Count > langCounts[j].Count
	})
	languages := make([]string, 0, len(langCounts))
	for _, langCount := range langCounts {
		languages = append(languages, langCount.Language)
	}

	return languages, nil
}

// readFile reads the file at the given path and returns its content.
func readFile(path string, limit int64) ([]byte, error) {
	if limit <= 0 {
		return os.ReadFile(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			log.Print(err)
		}
	}()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := st.Size()
	if size > limit {
		size = limit
	}
	buf := bytes.NewBuffer(nil)
	buf.Grow(int(size))
	_, err = io.Copy(buf, io.LimitReader(f, limit))
	return buf.Bytes(), err
}

// readIdeaDir reads .idea directory and tries to detect which languages are used in the project.
func readIdeaDir(project string) []string {
	var languages []string
	var files []string
	root := filepath.Join(project, ".idea")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return languages
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		files = append(files, path)
		return nil
	})
	if err != nil {
		panic(err)
	}
	for _, file := range files {
		if filepath.Ext(file) == ".iml" {
			iml, err := os.ReadFile(file)
			if err != nil {
				log.Fatal(err)
			}
			text := string(iml)
			if strings.Contains(text, "JAVA_MODULE") {
				languages = Append(languages, "Java")
			}
			if strings.Contains(text, "PYTHON_MODULE") {
				languages = Append(languages, "Python")
			}
			if strings.Contains(text, "Go") {
				languages = Append(languages, "Go")
			}
		}
	}
	return languages
}

// isAndroidProject checks if the given directory is an Android project by checking AndroidManifest
// https://developer.android.com/guide/topics/manifest/manifest-intro
func isAndroidProject(projectDir string) bool {
	var foundManifest bool
	err := filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), "AndroidManifest.xml") {
			foundManifest = true
			return filepath.SkipDir
		}

		return nil
	})
	if err != nil {
		log.Fatal("Error walking the path: ", err)
	}
	return foundManifest
}
