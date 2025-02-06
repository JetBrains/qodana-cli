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
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"os"
	"path/filepath"
)

func (l CdnetLinter) MountTools(tempMountPath string, mountPath string, _ bool) (map[string]string, error) {
	val := make(map[string]string)
	val[platform.Clt] = filepath.Join(mountPath, "tools", "netcoreapp3.1", "any", "JetBrains.CommandLine.Products.dll")
	archive := "clt.zip"
	if _, err := os.Stat(val["clt"]); err != nil {
		if os.IsNotExist(err) {
			path := platform.ProcessAuxiliaryTool(archive, "clang", tempMountPath, mountPath, Clt)
			if err := platform.Decompress(path, mountPath); err != nil {
				return nil, fmt.Errorf("failed to decompress clang archive: %w", err)
			}
		}
	}
	return val, nil
}
