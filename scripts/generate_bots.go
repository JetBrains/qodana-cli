//go:build ignore
// +build ignore

package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"text/template"
)

type BotList struct {
	Bots []string `json:"bots"`
}

//go:generate go run generate_bots.go
func main() {
	var botList BotList

	// Read botlist.json
	data, _ := ioutil.ReadFile("../bots.json")
	err := json.Unmarshal(data, &botList)
	if err != nil {
		return
	}

	// Define a template for the output Go file
	const goFileTemplate = `/*
 * Copyright 2021-2022 JetBrains s.r.o.
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

var gitHubBotSuffix = "[bot]@users.noreply.github.com"
var commonGitBots = []string{
    {{range .Bots}}"{{.}}",
    {{end}}
}
`

	// Parse the template and execute it, writing the output to bots.go
	tmpl, _ := template.New("test").Parse(goFileTemplate)
	file, _ := os.Create("../core/bots.go")
	err = tmpl.Execute(file, botList)
	if err != nil {
		return
	}
}
