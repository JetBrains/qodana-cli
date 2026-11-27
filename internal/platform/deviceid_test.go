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
	"testing"

	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/stretchr/testify/assert"
)

func TestDeviceIdAndRemoteUrl(t *testing.T) {
	t.Setenv(qdenv.QodanaRemoteUrl, "https://github.com/acme/app.git")
	t.Setenv("DEVICEID", "")

	deviceId, remoteUrl := DeviceIdAndRemoteUrl(t.TempDir())

	assert.Equal(t, GetDeviceIdSalt(t.TempDir())[0], deviceId)
	assert.Equal(t, "https://github.com/acme/app.git", remoteUrl)
}
