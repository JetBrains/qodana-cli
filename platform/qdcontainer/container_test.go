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

package qdcontainer

import (
	"context"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestClientCreationKeepsLogLevel(t *testing.T) {
	// There are a bunch of ways to initialize the Docker API and some of them produce unexpected side effects.
	for _, level := range log.AllLevels {
		log.SetLevel(level)
		_, err := NewContainerClient(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, log.GetLevel(), level)
	}
}
