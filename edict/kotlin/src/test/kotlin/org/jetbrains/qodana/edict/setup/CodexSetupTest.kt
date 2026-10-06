package org.jetbrains.qodana.edict.setup

import kotlinx.serialization.json.JsonPrimitive
import org.jetbrains.qodana.edict.common.EdictLayout
import org.junit.jupiter.api.Assumptions.assumeTrue
import org.junit.jupiter.api.io.TempDir
import org.junitpioneer.jupiter.ClearEnvironmentVariable
import org.tomlj.Toml
import java.nio.file.Path
import kotlin.io.path.createFile
import kotlin.io.path.exists
import kotlin.io.path.writeText
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class CodexSetupTest {
  @TempDir
  lateinit var directory: Path

  @TempDir
  lateinit var codexHome: Path

  @Test
  fun `install writes skills and a sandboxed local config`() {
    val project = EdictLayout(directory.toRealPath())
    val gold = project.root.resolve("gold.sarif.json").createFile()
    trust(project.root)

    val skills = CodexSetup.install(
      project, listOf(Path.of("gold.sarif.json"), Path.of("missing/../benchmark")), EdictConfig(4321), codexHome,
    )

    assertTrue("edict_manager" in skills)
    assertTrue(project.codexSkillsDirectory.resolve("edict_manager/SKILL.md").exists())
    val config = Toml.parse(project.codexConfigPath)
    assertTrue(config.errors().isEmpty(), config.errors().toString())
    assertEquals("edict", config.getString("default_permissions"))
    assertEquals(":read-only", config.getString(listOf("permissions", "edict", "extends")))
    val filesystem = config.getTable(listOf("permissions", "edict", "filesystem"))!!
    assertEquals(
      mapOf(
        ":tmpdir" to "write",
        project.root.toString() to "read",
        project.agentWorkRoot.toString() to "write",
        project.processLogRoot.toString() to "deny",
        gold.toString() to "deny",
        project.root.resolve("benchmark").toString() to "deny",
      ),
      filesystem.keySet().associateWith { filesystem.getString(listOf(it)) },
    )
    assertEquals("http://127.0.0.1:4321/mcp", config.getString(listOf("mcp_servers", "edict-mcp", "url")))
    CodexSetup.requireInstalled(project, EdictConfig(4321))
  }

  @Test
  fun `reinstall follows the configured port and drops deny rules that are no longer requested`() {
    val project = EdictLayout(directory.toRealPath())
    trust(project.root)
    CodexSetup.install(project, listOf(Path.of("gold.sarif.json")), EdictConfig(4321), codexHome)

    CodexSetup.install(project, emptyList(), EdictConfig(4322), codexHome)

    CodexSetup.requireInstalled(project, EdictConfig(4322))
    val filesystem = Toml.parse(project.codexConfigPath).getTable(listOf("permissions", "edict", "filesystem"))!!
    assertNull(filesystem.getString(listOf(project.root.resolve("gold.sarif.json").toString())))
  }

  @Test
  fun `server requires an installation for the configured port`() {
    val project = EdictLayout(directory.toRealPath())
    assertFailsWith<IllegalStateException> { CodexSetup.requireInstalled(project, EdictConfig()) }
    trust(project.root)
    CodexSetup.install(project, emptyList(), EdictConfig(4321), codexHome)
    val mismatch = assertFailsWith<IllegalStateException> { CodexSetup.requireInstalled(project, EdictConfig(4322)) }
    assertTrue("re-run `qodana edict install`" in mismatch.message!!)
  }

  @Test
  fun `install fails and removes the config when Codex does not load it`() {
    val project = EdictLayout(directory.toRealPath())
    // The fake Codex from build.gradle.kts answers with this list.
    codexHome.resolve("fake-mcp-list.json").writeText("[]")
    val ignored = assertFailsWith<IllegalStateException> { CodexSetup.install(project, emptyList(), EdictConfig(), codexHome) }
    assertTrue("trust_level = \"trusted\"" in ignored.message!!, ignored.message)
    assertFalse(project.codexConfigPath.exists())
  }

  /** The contract the fake stands in for: Codex loads `.codex/config.toml` only for a project trusted in `CODEX_HOME`. */
  @Test
  @ClearEnvironmentVariable(key = "CODEX_BIN")
  fun `real Codex loads the local config only for a trusted project`() {
    // CI must run this check; elsewhere it needs Codex on PATH.
    assumeTrue(System.getenv("CI") != null || System.getenv("TEAMCITY_VERSION") != null || codexInstalled()) {
      "codex is not on PATH"
    }
    val project = EdictLayout(directory.toRealPath())
    assertFailsWith<IllegalStateException> { CodexSetup.install(project, emptyList(), EdictConfig(), codexHome) }
    assertFalse(project.codexConfigPath.exists())

    codexHome.resolve("config.toml").writeText("[projects.${quote(project.root)}]\ntrust_level = \"untrusted\"\n")
    assertFailsWith<IllegalStateException> { CodexSetup.install(project, emptyList(), EdictConfig(), codexHome) }
    assertFalse(project.codexConfigPath.exists())

    trust(project.root)
    CodexSetup.install(project, emptyList(), EdictConfig(), codexHome)
    CodexSetup.requireInstalled(project, EdictConfig())
  }

  private fun trust(path: Path) {
    codexHome.resolve("config.toml").writeText("[projects.${quote(path)}]\ntrust_level = \"trusted\"\n")
  }
}

private fun quote(path: Path): String = JsonPrimitive(path.toString()).toString()

private fun codexInstalled(): Boolean =
  runCatching { ProcessBuilder("codex", "--version").redirectErrorStream(true).start().waitFor() == 0 }.getOrDefault(false)
