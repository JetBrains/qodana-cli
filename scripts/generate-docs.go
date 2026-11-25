//go:build ignore
// +build ignore

/*
 * Copyright 2021-2026 JetBrains s.r.o.
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

// Script to generate the documentation of the CLI commands in README.md from their --help. TestReadmeIsUpToDate of
// internal/cmd checks that README.md is up to date.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/JetBrains/qodana-cli/internal/cmd"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) > 1 {
		log.Fatalf("Unexpected arguments %v: the script regenerates README.md, TestReadmeIsUpToDate checks it", os.Args[1:])
	}

	readme, err := os.ReadFile("README.md")
	if err != nil {
		log.Fatalf("Run the script from the repository root: %v", err)
	}
	updated, err := cmd.GenerateReadme(string(readme))
	if err != nil {
		log.Fatal(err)
	}
	if updated == string(readme) {
		fmt.Println("README.md is up to date.")
		return
	}
	if err := os.WriteFile("README.md", []byte(updated), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Println("README.md updated.")
}
