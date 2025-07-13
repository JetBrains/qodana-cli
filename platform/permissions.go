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
	"os"
	"path/filepath"
	"runtime"
)

// ChangeResultsPermissionsRecursively changes the permissions of the given
// directory and all its contents to allow read and write
// permissions for files, and appropriate permissions for directories.
func ChangeResultsPermissionsRecursively(path string) error {
	//goland:noinspection GoBoolExpressions
	if runtime.GOOS == "windows" {
		return nil
	}
	return filepath.Walk(
		path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			var perm os.FileMode
			if info.IsDir() {
				// Set directory permissions to read, write, and
				// execute for owner and group, read and execute for others
				perm = 0775
			} else {
				// Set file permissions to read and write for owner, group, and others
				perm = 0666
			}

			err = os.Chmod(path, perm)
			if err != nil {
				return err
			}

			return nil
		},
	)
}

func ChangePermissionsRecursivelyUnix(path string, perm os.FileMode) error {
	return filepath.Walk(
		path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			err = os.Chmod(path, perm)
			if err != nil {
				return err
			}

			return nil
		},
	)
}
