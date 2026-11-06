// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import org.jetbrains.qodana.edict.edictnext.EdictNextJson
import org.jetbrains.qodana.edict.edictnext.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.feedback.support.feedbackRecord
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.io.path.createDirectories
import kotlin.io.path.writeText
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class FeedbackRecordsTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `refuses a record stored under another record's name`() {
    val paths = EdictRepositoryDirectory(directory)
    val file = paths.activeFeedbackDirectory.createDirectories().resolve("f-01.json")
    file.writeText(EdictNextJson.encodeToString(FeedbackRecord.serializer(), feedbackRecord(0)))

    val failure = assertFailsWith<IllegalArgumentException> { FeedbackRecords(paths).active() }

    assertEquals("Feedback record $file holds record f-00", failure.message)
  }
}
