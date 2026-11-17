// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.support

import java.net.URL

/** The test resource at [path], relative to the test resources root. */
internal fun resource(path: String): URL =
  requireNotNull(Thread.currentThread().contextClassLoader.getResource(path)) { "No test resource $path" }

/** The text of the test resource at [path], relative to the test resources root. */
internal fun resourceText(path: String): String = resource(path).readText()
