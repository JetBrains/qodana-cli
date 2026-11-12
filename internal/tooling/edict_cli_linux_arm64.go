//go:build linux && arm64

/*
 * Copyright 2026 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package tooling

import "embed"

//go:embed libs/linux-arm64/edict-cli-*.jar
var edictLib embed.FS
