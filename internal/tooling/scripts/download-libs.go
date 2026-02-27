//go:build ignore

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

package main

import (
	_ "embed"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed pom.xml
var QodanaLibsPom string

type Project struct {
	Repositories []Repository `xml:"repositories>repository"`
	Dependencies []Dependency `xml:"dependencies>dependency"`
}

type Repository struct {
	URL string `xml:"url"`
}

type Dependency struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

const libsDir = "libs"

func main() {
	project := parsePomXml()
	repoURL := project.Repositories[0].URL

	if err := os.MkdirAll(libsDir, 0o755); err != nil {
		log.Fatalf("Failed to create directory internal/tooling/%s: %v", libsDir, err)
	}

	for _, dep := range project.Dependencies {
		if checkArtifact(dep.ArtifactID, dep.Version) {
			log.Printf("OK, SKIPPED: %s-%s", dep.ArtifactID, dep.Version)
			continue
		}

		url := fmt.Sprintf(
			"%s/%s/%s/%s/%s-%s.jar",
			strings.TrimSuffix(repoURL, "/"),
			strings.ReplaceAll(dep.GroupID, ".", "/"),
			dep.ArtifactID,
			dep.Version,
			dep.ArtifactID,
			dep.Version,
		)

		log.Printf("DOWNLOADING: %s-%s", dep.ArtifactID, dep.Version)
		downloadFile(url)
	}
}

func checkArtifact(artifactID, artifactVersion string) (alreadyCorrect bool) {
	entries, err := os.ReadDir(libsDir)
	if err != nil {
		log.Fatalf("Failed to list directory %s: %v", libsDir, err)
	}

	prefix := artifactID + "-"
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".jar") {
			continue
		}

		fileVersion := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".jar")
		if fileVersion == "" {
			continue
		}

		fullPath := filepath.Join(libsDir, name)
		if fileVersion == artifactVersion {
			alreadyCorrect = true
			continue
		}

		log.Printf("UPDATING: %s %s -> %s", artifactID, fileVersion, artifactVersion)
		rmErr := os.Remove(fullPath)
		if rmErr != nil {
			log.Fatalf("Failed to remove %s: %v", fullPath, rmErr)
		}
	}

	return alreadyCorrect
}

func parsePomXml() Project {
	var project Project
	if err := xml.NewDecoder(strings.NewReader(QodanaLibsPom)).Decode(&project); err != nil {
		log.Fatalf("Could not decode pom.xml: %v", err)
	}

	if len(project.Repositories) == 0 {
		log.Fatalf("No repositories found in pom.xml")
	}
	return project
}

func downloadFile(url string) {
	resp, err := http.Get(url)
	if err != nil {
		log.Fatalf("Error downloading %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("bad status: %s", resp.Status)
	}

	dest := filepath.Join(libsDir, filepath.Base(url))
	out, err := os.Create(dest)
	if err != nil {
		log.Fatalf("Failed to create file %s: %v", dest, err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		log.Fatalf("Failed to write to %s: %v", dest, err)
	}
}
