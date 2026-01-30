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
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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

func main() {
	artifactId := flag.String("artifact", "", "Artifact ID to download")
	flag.Parse()
	if *artifactId == "" {
		log.Fatalf("the -artifact flag is required")
	}

	project := parsePomXml()
	repoURL := project.Repositories[0].URL

	artifactFound := false
	for _, dep := range project.Dependencies {
		if dep.ArtifactID != *artifactId {
			continue
		}
		sanitizedArtifactID := filepath.Base(dep.ArtifactID)
		sanitizedGroupID := filepath.Base(dep.GroupID)
		sanitizedVersion := filepath.Base(dep.Version)
		url := fmt.Sprintf(
			"%s/%s/%s/%s/%s-%s.jar",
			strings.TrimSuffix(repoURL, "/"),
			strings.ReplaceAll(sanitizedGroupID, ".", "/"),
			sanitizedArtifactID,
			sanitizedVersion,
			sanitizedArtifactID,
			sanitizedVersion,
		)

		destFile := filepath.Join(".", sanitizedArtifactID+".jar")
		log.Printf("Downloading %s to tooling/%s", url, destFile)

		if err := downloadFile(url, destFile); err != nil {
			log.Fatalf("Error downloading %s: %v", url, err)
		}
		artifactFound = true
	}

	if artifactFound == false {
		log.Fatalf("Requested artifact %s not found in pom.xml", *artifactId)
	}
}

func parsePomXml() Project {
	pomFileName := "pom.xml"

	_, err := os.Stat(pomFileName)
	if err != nil && os.IsNotExist(err) {
		log.Fatalf("internal/tooling/pom.xml does not exist: %v", err)
	}

	pomFile, err := os.Open(pomFileName)
	if err != nil {
		log.Fatalf("Could not open %s: %v", pomFileName, err)
	}
	defer pomFile.Close()

	var project Project
	if err := xml.NewDecoder(pomFile).Decode(&project); err != nil {
		log.Fatalf("Could not decode %s: %v", pomFileName, err)
	}

	if len(project.Repositories) == 0 {
		log.Fatalf("No repositories found in pom.xml")
	}
	return project
}

func downloadFile(url, dest string) error {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
