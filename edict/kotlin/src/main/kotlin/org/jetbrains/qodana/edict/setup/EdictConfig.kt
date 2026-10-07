// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.setup


import org.jetbrains.qodana.edict.common.DEFAULT_EDICT_MCP_PORT
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.common.EdictYamlConfiguration
import java.nio.file.Path

/** Settings from the `edict` section of the project's `qodana.yaml`; every key is optional. */
internal data class EdictConfig(val mcpPort: Int = DEFAULT_MCP_PORT) {
  init {
    require(mcpPort in 1..65535) { "edict.mcpPort must be between 1 and 65535: $mcpPort" }
  }

  companion object {
    /** Outside the ephemeral range, where the OS assigns ports to outgoing connections. */
    const val DEFAULT_MCP_PORT: Int = DEFAULT_EDICT_MCP_PORT

    fun load(layout: EdictLayout): EdictConfig = load(layout.qodanaYamlPath)

    fun load(path: Path?): EdictConfig = EdictConfig(EdictYamlConfiguration.load(path).mcpPort)
  }
}
