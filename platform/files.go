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
	"fmt"
	log "github.com/sirupsen/logrus"
	"path"
	"path/filepath"
	"reflect"
	"text/tabwriter"
	"unsafe"
)

func LogContext(contextPointer any) {
	buffer := new(bytes.Buffer)
	w := new(tabwriter.Writer)
	w.Init(buffer, 0, 8, 2, '\t', 0)

	_, err := fmt.Fprintln(w, "Option\tValue\t")
	if err != nil {
		return
	}
	_, err = fmt.Fprintln(w, "------\t-----\t")
	if err != nil {
		return
	}

	value := reflect.ValueOf(contextPointer).Elem()
	typeInfo := value.Type()

	for i := 0; i < value.NumField(); i++ {
		fieldType := typeInfo.Field(i)

		fieldValue := value.Field(i)
		// unexported fields
		fieldValue = reflect.NewAt(fieldValue.Type(), unsafe.Pointer(fieldValue.UnsafeAddr())).Elem()

		line := fmt.Sprintf("%s\t%v\t", fieldType.Name, fieldValue.Interface())
		_, err = fmt.Fprintln(w, line)
		if err != nil {
			return
		}
	}
	if err := w.Flush(); err != nil {
		return
	}
	log.Debug(buffer.String())
}

func ReportResultsPath(reportDir string) string {
	return filepath.Join(reportDir, "results")
}

func GetTmpResultsDir(resultsDir string) string {
	return path.Join(resultsDir, "tmp")
}

func GetCoverageArtifactsPath(resultsDir string) string {
	return path.Join(resultsDir, "coverage")
}

func GetSarifPath(resultsDir string) string {
	return path.Join(resultsDir, "qodana.sarif.json")
}

func GetShortSarifPath(resultsDir string) string {
	return path.Join(resultsDir, "qodana-short.sarif.json")
}
