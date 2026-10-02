package org.jetbrains.qodana.edict.edictnext

import org.junitpioneer.jupiter.ClearEnvironmentVariable
import org.junitpioneer.jupiter.SetEnvironmentVariable
import org.junitpioneer.jupiter.SetSystemProperty
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class EdictNextPathsTest {
  @Test
  @SetSystemProperty(key = "os.name", value = "Mac OS X")
  @SetEnvironmentVariable(key = "HOME", value = "/home/user")
  fun `uses Library Caches on macOS`() {
    assertEquals(Path.of("/home/user/Library/Caches"), userCacheDirectory())
  }

  @Test
  @SetSystemProperty(key = "os.name", value = "Linux")
  @SetEnvironmentVariable(key = "HOME", value = "/home/user")
  @SetEnvironmentVariable(key = "XDG_CACHE_HOME", value = "/xdg")
  fun `prefers XDG_CACHE_HOME on Linux`() {
    assertEquals(Path.of("/xdg"), userCacheDirectory())
  }

  @Test
  @SetSystemProperty(key = "os.name", value = "Linux")
  @SetEnvironmentVariable(key = "HOME", value = "/home/user")
  @SetEnvironmentVariable(key = "XDG_CACHE_HOME", value = "")
  fun `falls back to dot cache on Linux`() {
    assertEquals(Path.of("/home/user/.cache"), userCacheDirectory())
  }

  @Test
  @SetSystemProperty(key = "os.name", value = "Linux")
  @SetEnvironmentVariable(key = "XDG_CACHE_HOME", value = "relative")
  fun `rejects a relative XDG_CACHE_HOME`() {
    assertFailsWith<IllegalArgumentException> { userCacheDirectory() }
  }

  @Test
  @SetSystemProperty(key = "os.name", value = "Windows 11")
  @SetEnvironmentVariable(key = "LocalAppData", value = "C:\\Users\\user\\AppData\\Local")
  fun `uses LocalAppData on Windows`() {
    assertEquals(Path.of("C:\\Users\\user\\AppData\\Local"), userCacheDirectory())
  }

  @Test
  @SetSystemProperty(key = "os.name", value = "Windows 11")
  @ClearEnvironmentVariable(key = "LocalAppData")
  fun `rejects an undefined LocalAppData`() {
    assertFailsWith<IllegalStateException> { userCacheDirectory() }
  }
}
