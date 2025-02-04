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
	"github.com/JetBrains/qodana-cli/v2024/platform/tokenloader"
)

func (o *QodanaOptions) LoadToken(refresh bool, requiresToken bool, interactive bool) string {
	return tokenloader.LoadCloudToken(o.AsInitOptions(), refresh, requiresToken, interactive)
}

// ValidateToken checks if QODANA_TOKEN is set in CLI args, or environment or the system keyring, returns its value.
func (o *QodanaOptions) ValidateToken(refresh bool) string {
	return tokenloader.ValidateToken(o.AsInitOptions(), refresh)
}
