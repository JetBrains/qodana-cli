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
	"crypto/md5"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdenv"
	"os"
	"os/exec"
	"strings"
)

// GetDeviceIdSalt set consistent device.id based on given repo upstream #SA-391.
func GetDeviceIdSalt() []string {
	salt := os.Getenv("SALT")
	deviceId := os.Getenv("DEVICEID")
	if salt == "" || deviceId == "" {
		hash := "00000000000000000000000000000000"
		remoteUrl := getRemoteUrl()
		if remoteUrl != "" {
			hash = fmt.Sprintf("%x", md5.Sum(append([]byte("1n1T-$@Lt-"), remoteUrl...)))
		}
		if salt == "" {
			salt = fmt.Sprintf("%x", md5.Sum([]byte("$eC0nd-$@Lt-"+hash)))
		}
		if deviceId == "" {
			deviceId = fmt.Sprintf("200820300000000-%s-%s-%s-%s", hash[0:4], hash[4:8], hash[8:12], hash[12:24])
		}
	}
	return []string{deviceId, salt}
}

// getRemoteUrl returns remote url of the current git repository.
func getRemoteUrl() string {
	url := os.Getenv(qdenv.QodanaRemoteUrl)
	if url == "" {
		out, err := exec.Command("git", "remote", "get-url", "origin").Output()
		if err != nil {
			return ""
		}
		url = string(out)
	}
	return strings.TrimSpace(url)
}
