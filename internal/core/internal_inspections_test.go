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
	"testing"

	"github.com/JetBrains/qodana-cli/internal/core/corescan"
	"github.com/JetBrains/qodana-cli/internal/foundation/fs"
	"github.com/JetBrains/qodana-cli/internal/platform/product"
	"github.com/JetBrains/qodana-cli/internal/platform/qdcontainer"
	"github.com/JetBrains/qodana-cli/internal/platform/qdenv"
	"github.com/docker/docker/api/types/mount"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerReadsInternalInspectionsAtTheirMount(t *testing.T) {
	ctx := containerContext(t, t.TempDir())

	var properties []string
	for _, argument := range GetIdeArgs(ctx) {
		if strings.HasPrefix(argument, "--property=") {
			properties = append(properties, argument)
		}
	}

	assert.Equal(
		t,
		[]string{"--property=idea.log.level=DEBUG", "--property=" + internalInspectionsDirProperty + "=" + qdcontainer.DataInternalInspectionsDir},
		properties,
	)
}

func TestContainerMountsTheInternalInspectionsDirectory(t *testing.T) {
	dir := t.TempDir()
	canonical, err := fs.Canonical(dir)
	require.NoError(t, err)

	mounts := getDockerOptions(containerContext(t, dir), "jetbrains/qodana-jvm:latest").HostConfig.Mounts

	assert.Contains(t, mounts, mount.Mount{Type: mount.TypeBind, Source: canonical, Target: qdcontainer.DataInternalInspectionsDir})
}

func TestContainerMountsNoInternalInspectionsWithoutTheProperty(t *testing.T) {
	builder := containerBuilder(t)
	builder.Property = []string{"idea.log.level=DEBUG"}

	mounts := getDockerOptions(builder.Build(), "jetbrains/qodana-jvm:latest").HostConfig.Mounts

	for _, m := range mounts {
		assert.NotEqual(t, qdcontainer.DataInternalInspectionsDir, m.Target)
	}
}

// containerContext is a container run whose properties name the internal inspections directory dir among others.
func containerContext(t *testing.T, dir string) corescan.Context {
	builder := containerBuilder(t)
	builder.Property = []string{"idea.log.level=DEBUG", internalInspectionsDirProperty + "=" + dir}
	return builder.Build()
}

// containerBuilder is a container run of the JVM linter with existing project, cache and results directories.
func containerBuilder(t *testing.T) corescan.ContextBuilder {
	qdenv.InitializeQodanaGlobalEnv(qdenv.EmptyEnvProvider())
	root := t.TempDir()
	return corescan.ContextBuilder{
		ProjectDir:     root,
		RepositoryRoot: root,
		CacheDir:       t.TempDir(),
		ResultsDir:     t.TempDir(),
		ReportDir:      t.TempDir(),
		Analyser:       product.JvmLinter.DockerAnalyzer(),
		Prod:           product.Product{Version: "2025.3"},
	}
}
