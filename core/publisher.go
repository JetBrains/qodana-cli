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

/*
 * This file contains the code for sending the report to Qodana Cloud.
 * The publisher is a part of Qodana linters.
 * This will be refactored/removed after the proper endpoint is implemented.
 */

package core

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"github.com/pterm/pterm"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

type metadata struct {
	Versioning versioning `xml:"versioning"`
}

type versioning struct {
	Latest  string `xml:"latest"`
	Release string `xml:"release"`
}

// sendReport sends report to Qodana Cloud.
func sendReport(opts *QodanaOptions, token string) {
	path := Prod.ideBin()
	if !IsContainer() {
		path = opts.confDirPath()
		fetchPublisher(path)
	}
	publisher := filepath.Join(path, "publisher.jar")
	if _, err := os.Stat(publisher); os.IsNotExist(err) {
		log.Fatalf("Not able to send the report: %s is missing", publisher)
	}
	copyReportInNativeMode(opts)
	publisherCommand := getPublisherArgs(publisher, opts, token, os.Getenv(qodanaEndpoint))
	if res := RunCmd("", publisherCommand...); res > 0 {
		os.Exit(res)
	}
}

func publisherVersion() versioning {
	resp, err := http.Get("https://packages.jetbrains.team/maven/p/ij/intellij-dependencies/org/jetbrains/qodana/publisher/maven-metadata.xml")
	if err != nil {
		log.Fatal(err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(resp.Body)

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	meta := &metadata{}
	err = xml.Unmarshal(content, meta)
	if err != nil {
		log.Fatal(err)
	}
	return meta.Versioning
}

func getPublisherUrl(version string) string {
	return "https://packages.jetbrains.team/maven/p/ij/intellij-dependencies/org/jetbrains/qodana/publisher-cli/" + version + "/publisher-cli-" + version + ".jar"
}

func fetchPublisher(directory string) {
	version := publisherVersion().Release
	path := filepath.Join(directory, "publisher.jar")
	if _, err := os.Stat(path); err == nil {
		return
	}
	err := downloadFile(path, getPublisherUrl(version), nil)
	if err != nil {
		log.Fatal(err)
	}
	verifyMd5Hash(version, path)
}

func verifyMd5Hash(version string, path string) {
	if _, err := os.Stat(path); err != nil {
		log.Fatal(err)
	}
	url := getPublisherUrl(version) + ".md5"
	resp, err := http.Get(url)
	if err != nil {
		log.Fatalf("Error downloading md5 hash: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading md5 hash: %v", err)
	}

	downloadedMd5 := string(body)
	fileContent, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Error reading file: %v", err)
	}

	hasher := md5.New()
	_, err = hasher.Write(fileContent)
	if err != nil {
		log.Fatalf("Error computing md5 hash: %v", err)
	}

	computedMd5 := hex.EncodeToString(hasher.Sum(nil))

	if computedMd5 != downloadedMd5 {
		err = os.Remove(path)
		if err != nil {
			log.Fatalf("Please remove file, since md5 doesn't match: %s", path)
		}
		log.Fatal("The provided file and the file from the link have different md5 hashes")
	} else {
		println("Obtained publisher " + version + " and successfully checked md5 hash")
	}
}

func downloadFile(filepath string, url string, spinner *pterm.SpinnerPrinter) error {
	response, err := http.Head(url)
	if err != nil {
		return err
	}
	size, _ := strconv.Atoi(response.Header.Get("Content-Length"))

	resp, err := http.Get(url)
	if err != nil {
		return err
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Fatalf("Error while closing HTTP stream: %v", err)
		}
	}(resp.Body)

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}

	defer func(out *os.File) {
		err := out.Close()
		if err != nil {
			log.Fatalf("Error while closing output file: %v", err)
		}
	}(out)

	buffer := make([]byte, 1024)
	total := 0
	lastTotal := 0
	text := ""
	if spinner != nil {
		text = spinner.Text
	}
	for {
		length, err := resp.Body.Read(buffer)
		if err != nil && err != io.EOF {
			return err
		}
		total += length
		if spinner != nil && total-lastTotal > 1024*1024 {
			lastTotal = total
			spinner.UpdateText(fmt.Sprintf("%s (%d %%)", text, 100*total/size))
		}
		if length == 0 {
			break
		}
		if _, err = out.Write(buffer[:length]); err != nil {
			return err
		}
	}

	if total != size {
		return fmt.Errorf("downloaded file size doesn't match expected size")
	}

	if spinner != nil {
		spinner.UpdateText(fmt.Sprintf("%s (100 %%)", text))
	}

	return nil
}
