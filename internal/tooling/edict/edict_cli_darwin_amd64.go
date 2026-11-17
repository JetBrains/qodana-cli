//go:build darwin && amd64

/*
 * Copyright 2026 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package edict

import "embed"

//go:embed libs/darwin-amd64/edict-cli-*.jar
var EdictLib embed.FS
