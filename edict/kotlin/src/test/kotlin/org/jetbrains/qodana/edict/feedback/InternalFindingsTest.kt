// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import org.jetbrains.qodana.edict.edictnext.EdictNextLineRange
import org.jetbrains.qodana.edict.feedback.support.FINDING_MESSAGE
import org.jetbrains.qodana.edict.feedback.support.HEAD_REVISION
import org.jetbrains.qodana.edict.feedback.support.internalResult
import org.jetbrains.qodana.edict.feedback.support.sarifReport
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.io.path.writeText
import kotlin.test.assertEquals

class InternalFindingsTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `reads the problems of the inspections run for feedback, by cluster and repository path`() {
    val report = sarifReport(
      internalResult("rule", "src/Main.kt", startLine = 3, endLine = 4),
      internalResult("other", "src/Other.kt", startLine = 7),
      // Without the trailing separator, as Qodana may write a directory.
      projectDirectory = "service",
    )

    assertEquals(
      listOf(
        InternalFinding("rule", "service/src/Main.kt", EdictNextLineRange(3, 4), FINDING_MESSAGE),
        InternalFinding("other", "service/src/Other.kt", EdictNextLineRange(7, 7), FINDING_MESSAGE),
      ),
      read(report).findings,
    )
  }

  @Test
  fun `the report names the commit it analysed`() {
    assertEquals(HEAD_REVISION, read(sarifReport(internalResult("rule", "src/Main.kt", startLine = 3))).revision)
  }

  @Test
  fun `leaves out a problem of a whole file, which has no line to comment on`() {
    val report = sarifReport(internalResult("rule", "src/Main.kt", startLine = null), internalResult("rule", "src/Main.kt", startLine = 3))

    assertEquals(listOf(InternalFinding("rule", "src/Main.kt", EdictNextLineRange(3, 3), FINDING_MESSAGE)), read(report).findings)
  }

  @Test
  fun `a report whose inspections run for feedback found nothing has no findings`() {
    // Qodana leaves the property out when those inspections report nothing.
    assertEquals(FeedbackReport("", emptyList()), read("""{"runs": [{"results": [], "properties": {}}]}"""))
  }

  private fun read(report: String): FeedbackReport =
    feedbackReport(directory.resolve("qodana.sarif.json").apply { writeText(report) })
}
