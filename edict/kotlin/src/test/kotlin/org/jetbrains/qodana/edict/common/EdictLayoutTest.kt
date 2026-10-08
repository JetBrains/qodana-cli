// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.common

import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertEquals

class EdictLayoutTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `resolves relative state path from project root`() {
    val layout = EdictLayout.get("state/edict")

    assertEquals(layout.root.resolve("state/edict"), layout.stateDirectory)
  }

  @Test
  fun `preserves absolute state path`() {
    val state = directory.resolve("external-edict-state").toAbsolutePath().normalize()

    assertEquals(state, EdictLayout.get(state.toString()).stateDirectory)
  }

  @Test
  fun `keeps IntelliJ MCP results beside its process log`() {
    val log = directory.resolve("logs")
    val layout = EdictLayout(directory, directory.resolve("state"), log, "test-run")

    assertEquals(log.resolve("process-log/test-run/intellij-mcp.log"), layout.intellijMcpLogPath)
    assertEquals(log.resolve("process-log/test-run/intellij-mcp"), layout.intellijMcpResultsDirectory)
  }
}
