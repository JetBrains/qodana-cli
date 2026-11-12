/*
 * Copyright 2026 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package tooling

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"runtime"
	"strings"
	"testing"
)

func TestEmbeddedEdictContainsOnlyTargetNatives(t *testing.T) {
	// The portable JAR is an input to jdeps, never an embedded common library.
	portable, err := fs.Glob(libs, "libs/edict-cli-*.jar")
	if err != nil || len(portable) != 0 {
		t.Fatalf("portable Edict JAR embedded in common libraries: %v, %v", portable, err)
	}
	name := findLibFile(edictLib, EdictCli)
	target := runtime.GOOS + "-" + runtime.GOARCH
	if !strings.HasPrefix(name, "libs/"+target+"/") {
		t.Fatalf("Edict JAR %s does not match %s", name, target)
	}
	data, err := fs.ReadFile(edictLib, name)
	if err != nil {
		t.Fatal(err)
	}
	jar, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	nativePlatform := map[string]string{
		"darwin-amd64": "osx-x64", "darwin-arm64": "osx-aarch64",
		"linux-amd64": "linux-x64", "linux-arm64": "linux-aarch64",
		"windows-amd64": "win-x64", "windows-arm64": "",
	}[target]
	natives := 0
	for _, entry := range jar.File {
		if entry.FileInfo().IsDir() || !strings.HasPrefix(entry.Name, "ai/onnxruntime/native/") {
			continue
		}
		if nativePlatform == "" || !strings.HasPrefix(entry.Name, "ai/onnxruntime/native/"+nativePlatform+"/") {
			t.Errorf("foreign native library embedded: %s", entry.Name)
		}
		if strings.HasSuffix(entry.Name, ".pdb") || strings.Contains(entry.Name, ".dSYM/") {
			t.Errorf("native debug symbols embedded: %s", entry.Name)
		}
		natives++
	}
	if nativePlatform != "" && natives == 0 {
		t.Fatal("target native libraries missing from embedded Edict JAR")
	}
	for _, resource := range []string{
		"META-INF/MANIFEST.MF",
		"org/jetbrains/qodana/edict/MainKt.class",
		"ai/onnxruntime/OrtEnvironment.class",
		"skills/index.txt",
	} {
		if _, err := fs.Stat(jar, resource); err != nil {
			t.Errorf("required Edict resource %s missing: %v", resource, err)
		}
	}
}
