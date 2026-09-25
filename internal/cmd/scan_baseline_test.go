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

package cmd

import (
	"path/filepath"
	"testing"

	"github.com/JetBrains/qodana-cli/internal/core/corescan"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/stretchr/testify/assert"
)

func TestBaselineForLinter(t *testing.T) {
	cacheDir := filepath.Join("/home", "user", ".cache", "qodana", "jvm")
	downloaded := filepath.Join(cacheDir, "cloud-baseline-42", "qodana.sarif.json")
	// a linter in a container reads the cache dir at its mount point
	t.Run("container run", func(t *testing.T) {
		context := corescan.ContextBuilder{
			CacheDir: cacheDir,
			Analyser: &product.DockerAnalyzer{Linter: product.JvmLinter, Image: product.JvmLinter.DockerImage},
		}.Build()

		assert.Equal(
			t,
			"/data/cache/cloud-baseline-42/qodana.sarif.json",
			baselineForLinter(downloaded, context),
		)
	})

	t.Run("native run", func(t *testing.T) {
		context := corescan.ContextBuilder{
			CacheDir: cacheDir,
			Analyser: &product.NativeAnalyzer{Linter: product.JvmLinter},
		}.Build()

		assert.Equal(t, downloaded, baselineForLinter(downloaded, context))
	})
}
