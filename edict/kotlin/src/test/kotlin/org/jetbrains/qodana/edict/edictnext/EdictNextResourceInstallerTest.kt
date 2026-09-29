package org.jetbrains.qodana.edict.edictnext

import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Files
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertTrue

class EdictNextResourceInstallerTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `bundles retrieval script resources`() {
    assertTrue(EdictScriptRunner.EdictNextResourceInstaller.readEdictNextResource("/edict-next/cluster.py").isNotEmpty())
    assertTrue(EdictScriptRunner.EdictNextResourceInstaller.readEdictNextResource("/edict-next/requirements.txt").isNotEmpty())
  }

  @Test
  fun `uses a prepared embedding Python without installing another environment`() = runBlocking {
    val python = Files.writeString(directory.resolve("embedding-python"), "prepared")
    assertTrue(python.toFile().setExecutable(true))
    EdictScriptRunner(EdictNextWorkspace(directory.resolve("workspace")), python).run {
      prepareEnvironment()
      deleteEnvironment()
    }
  }
}
