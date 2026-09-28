package org.jetbrains.qodana.edict.edictnext

import org.junit.jupiter.api.io.TempDir
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
}
