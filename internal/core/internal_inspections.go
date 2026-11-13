/*
 * Copyright 2021-2026 JetBrains s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package core

import (
	"strings"

	"github.com/JetBrains/qodana-cli/internal/core/corescan"
	"github.com/JetBrains/qodana-cli/internal/platform/qdcontainer"
)

// internalInspectionsDirProperty names the directory of the inspections the IDE runs without reporting their findings
// anywhere but in the SARIF run properties. Callers give the host directory; a container reads it at its mount.
const internalInspectionsDirProperty = "qodana.internal.inspections.kts.dir"

// internalInspectionsDir is the host directory that the property names, or "" when no property names one.
func internalInspectionsDir(c corescan.Context) string {
	properties, _ := c.PropertiesAndFlags()
	return properties[internalInspectionsDirProperty]
}

// withContainerInternalDirProperty is the properties of c with the internal inspections directory at its container mount
// instead of on the host.
func withContainerInternalDirProperty(properties []string) []string {
	for i, property := range properties {
		if strings.HasPrefix(property, internalInspectionsDirProperty+"=") {
			properties[i] = internalInspectionsDirProperty + "=" + qdcontainer.DataInternalInspectionsDir
		}
	}
	return properties
}
