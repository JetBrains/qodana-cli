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

package platforminit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContainsUnityProjects(t *testing.T) {
	tt := []struct {
		name         string
		want         bool
		setupProject func(dir string) error
	}{
		{
			name: "valid unity project",
			want: true,
			setupProject: func(dir string) error {
				assetsPath := filepath.Join(dir, "Assets")
				projectSettingsPath := filepath.Join(dir, "ProjectSettings")
				_ = os.MkdirAll(assetsPath, os.ModePerm)
				_ = os.MkdirAll(projectSettingsPath, os.ModePerm)
				unityFile := filepath.Join(projectSettingsPath, "ProjectVersion.txt")
				_ = os.WriteFile(unityFile, []byte{}, os.ModePerm)
				return nil
			},
		},
		{
			name: "missing assets dir",
			want: false,
			setupProject: func(dir string) error {
				projectSettingsPath := filepath.Join(dir, "ProjectSettings")
				_ = os.MkdirAll(projectSettingsPath, os.ModePerm)
				unityFile := filepath.Join(projectSettingsPath, "ProjectVersion.txt")
				_ = os.WriteFile(unityFile, []byte{}, os.ModePerm)
				return nil
			},
		},
		{
			name: "missing project settings",
			want: false,
			setupProject: func(dir string) error {
				assetsPath := filepath.Join(dir, "Assets")
				_ = os.MkdirAll(assetsPath, os.ModePerm)
				return nil
			},
		},
		{
			name: "empty project dir",
			want: false,
			setupProject: func(dir string) error {
				return nil
			},
		},
	}

	for _, tc := range tt {
		t.Run(
			tc.name, func(t *testing.T) {
				dir, err := os.MkdirTemp("", "unity-project")
				if err != nil {
					t.Fatalf("Error creating tmp dir: %v", err)
				}
				defer func(path string) {
					err := os.RemoveAll(path)
					if err != nil {
						t.Fatalf("Error removing tmp dir: %v", err)
					}
				}(dir)

				err = tc.setupProject(dir)
				if err != nil {
					t.Fatalf("Error setting up test project: %v", err)
				}

				got := containsUnityProjects(dir)
				if got != tc.want {
					t.Fatalf("expected %v, got %v", tc.want, got)
				}
			},
		)
	}
}

//goland:noinspection HttpUrlsUsage
const v45ProjectOld = `<?xml version="1.0" encoding="utf-8"?>
<Project ToolsVersion="12.0" DefaultTargets="Build" xmlns="http://schemas.microsoft.com/developer/msbuild/2003">
  <Import Project="$(MSBuildExtensionsPath)\$(MSBuildToolsVersion)\Microsoft.Common.props" Condition="Exists('$(MSBuildExtensionsPath)\$(MSBuildToolsVersion)\Microsoft.Common.props')" />
  <PropertyGroup>
    <TargetFrameworkVersion>v4.5</TargetFrameworkVersion>
  </PropertyGroup>
</Project>`

const v45ProjectNew = `<Project Sdk="Microsoft.NET.Sdk">
	  <PropertyGroup>
		<TargetFramework>net45</TargetFramework>
	  </PropertyGroup>
	</Project>`

const net5Project = `<Project Sdk="Microsoft.NET.Sdk">
	  <PropertyGroup>
		<TargetFramework>net5.0</TargetFramework>
	  </PropertyGroup>
	</Project>`

func TestContainsDotNetProjects(t *testing.T) {
	var testCases = []struct {
		name     string
		files    []string
		expected bool
		old      bool
	}{
		{
			name:     "NoFiles",
			files:    []string{},
			expected: false,
		},
		{
			name:     "NonDotnetFiles",
			files:    []string{"file.txt", "project.docx", "non-csproj.mono"},
			expected: false,
		},
		{
			name:     "v45New",
			files:    []string{"file1.csproj"},
			expected: true,
			old:      false,
		},
		{
			name:     "v45Old",
			files:    []string{"file1.csproj"},
			expected: true,
			old:      true,
		},
		{
			name:     "net5",
			files:    []string{"file1.csproj", "file2.txt", "file3.docx"},
			expected: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(
			testCase.name, func(t *testing.T) {
				projectDir, _ := os.MkdirTemp("", "gotestdotnetprojects")
				for _, file := range testCase.files {
					content := ""
					if strings.HasSuffix(file, ".csproj") {
						content = net5Project
						if testCase.expected {
							if testCase.old {
								content = v45ProjectOld
							} else {
								content = v45ProjectNew
							}
						}
					}

					err := os.WriteFile(projectDir+"/"+file, []byte(content), 0755)
					if err != nil {
						t.Fatalf("Failed to create file in test directory: %v", err)
					}
				}

				result := containsDotNetFxProjects(projectDir)
				_ = os.RemoveAll(projectDir)
				if result != testCase.expected {
					t.Errorf("Got %v, expected %v", result, testCase.expected)
				}
			},
		)
	}
}
