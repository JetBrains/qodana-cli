package org.jetbrains.qodana.edict.setup

import org.jetbrains.qodana.edict.common.EdictLayout
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.io.path.writeText
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class EdictConfigTest {
  @TempDir
  lateinit var directory: Path

  private val project: EdictLayout get() = EdictLayout(directory, directory.resolve(".edict"))

  @Test
  fun `defaults without qodana yaml, an edict section or a port`() {
    assertEquals(EdictConfig(EdictConfig.DEFAULT_MCP_PORT), EdictConfig.load(project))
    for (text in listOf("", "version: \"1.0\"\nlinter: jetbrains/qodana-jvm\n", "edict:\n", "edict: {}\n")) {
      directory.resolve("qodana.yaml").writeText(text)
      assertEquals(EdictConfig(EdictConfig.DEFAULT_MCP_PORT), EdictConfig.load(project), text)
    }
  }

  @Test
  fun `reads the edict section and ignores the rest of qodana yaml`() {
    directory.resolve("qodana.yaml").writeText(
      """
      version: "1.0"
      profile:
        name: qodana.recommended
      edict:
        mcpPort: 4321
        statePath: ../shared-edict-state
      """.trimIndent(),
    )
    assertEquals(EdictConfig(4321), EdictConfig.load(project))
  }

  @Test
  fun `prefers qodana yml like the Qodana CLI`() {
    directory.resolve("qodana.yaml").writeText("edict:\n  mcpPort: 4321\n")
    directory.resolve("qodana.yml").writeText("edict:\n  mcpPort: 4322\n")
    assertEquals(EdictConfig(4322), EdictConfig.load(project))
  }

  @Test
  fun `rejects invalid edict settings`() {
    for (section in listOf("mcpPort: 0", "mcpPort: 70000", "mcpPort: port", "mcpPorts: 4321")) {
      directory.resolve("qodana.yaml").writeText("edict:\n  $section\n")
      assertFailsWith<IllegalArgumentException>(section) { EdictConfig.load(project) }
    }
  }
}
