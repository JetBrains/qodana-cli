// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.setup


import com.charleskorn.kaml.Yaml
import com.charleskorn.kaml.YamlConfiguration
import com.charleskorn.kaml.YamlMap
import com.charleskorn.kaml.YamlNode
import com.charleskorn.kaml.YamlNull
import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.common.EdictLayout
import kotlin.io.path.readText

/** Settings from the `edict` section of the project's `qodana.yaml`; every key is optional. */
@Serializable
internal data class EdictConfig(val mcpPort: Int = DEFAULT_MCP_PORT) {
  init {
    require(mcpPort in 1..65535) { "edict.mcpPort must be between 1 and 65535: $mcpPort" }
  }

  companion object {
    /** Outside the ephemeral range, where the OS assigns ports to outgoing connections. */
    const val DEFAULT_MCP_PORT: Int = 27182

    fun load(layout: EdictLayout): EdictConfig {
      val path = layout.qodanaYamlPath ?: return EdictConfig()
      val text = path.readText()
      if (text.isBlank()) return EdictConfig()
      val section = try {
        (yaml.parseToYamlNode(text) as? YamlMap)?.get<YamlNode>("edict")
      }
      catch (e: Exception) {
        throw IllegalArgumentException("Cannot parse $path: ${e.message}", e)
      }
      if (section == null || section is YamlNull) return EdictConfig()
      return try {
        yaml.decodeFromYamlNode(serializer(), section)
      }
      catch (e: Exception) {
        throw IllegalArgumentException("Invalid edict section in $path: ${e.message}", e)
      }
    }
  }
}

// Other qodana.yaml sections belong to Qodana; unknown keys inside `edict` are still rejected.
private val yaml = Yaml(configuration = YamlConfiguration(strictMode = true))
