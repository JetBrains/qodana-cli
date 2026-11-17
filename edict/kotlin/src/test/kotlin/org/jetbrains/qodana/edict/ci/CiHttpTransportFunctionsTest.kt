// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci

import kotlin.test.Test
import kotlin.test.assertEquals

/** [urlPart] is form encoding, which the JDK does, except for the space. */
internal class CiHttpTransportFunctionsTest {
  @Test
  fun `a space is percent-encoded in a URL part, not written as a plus`() {
    assertEquals("is%3Apr%20is%3Amerged", urlPart("is:pr is:merged"))
  }
}
