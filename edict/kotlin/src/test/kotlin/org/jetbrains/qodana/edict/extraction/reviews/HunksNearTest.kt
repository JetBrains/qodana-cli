package org.jetbrains.qodana.edict.extraction.reviews

import kotlin.test.Test
import kotlin.test.assertEquals

class HunksNearTest {
  private val header = "--- a/src/A.kt\n+++ b/src/A.kt\n"
  private val first = "@@ -10,3 +10,3 @@\n a\n-b\n+c\n"
  private val insertion = "@@ -50,0 +51,1 @@\n+d\n"
  private val last = "@@ -200,2 +201,2 @@ fun f()\n e\n-f\n+g\n"
  private val diff = header + first + insertion + last

  @Test
  fun `keeps the headers and the hunks whose before lines overlap the range`() {
    assertEquals(header + first, hunksNear(diff, 1..10))
    assertEquals(header + insertion, hunksNear(diff, 50..60))
    assertEquals(header + first + insertion + last, hunksNear(diff, 12..200))
  }

  @Test
  fun `keeps only the headers when no hunk is near`() {
    assertEquals(header, hunksNear(diff, 100..150))
  }
}
