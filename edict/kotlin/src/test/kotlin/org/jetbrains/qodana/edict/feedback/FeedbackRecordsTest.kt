// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import org.jetbrains.qodana.edict.edictnext.EdictNextJson
import org.jetbrains.qodana.edict.edictnext.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.feedback.support.feedbackRecord
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.io.path.createDirectories
import kotlin.io.path.readText
import kotlin.io.path.writeText
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class FeedbackRecordsTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `a record moves to the history of the month it was reserved in`() {
    val paths = EdictRepositoryDirectory(directory)
    val record = feedbackRecord(1)
    val records = FeedbackRecords(paths)
    records.addActive(record)

    records.moveToHistory(record)

    assertEquals(emptyList(), records.active())
    // The record was reserved on 2026-11-05.
    assertEquals(record.fileText(), paths.feedbackHistoryDirectory.resolve("2026-11/f-01.json").readText())
  }

  @Test
  fun `refuses a record stored under another record's name`() {
    val paths = EdictRepositoryDirectory(directory)
    val file = paths.activeFeedbackDirectory.createDirectories().resolve("f-01.json")
    file.writeText(EdictNextJson.encodeToString(FeedbackRecord.serializer(), feedbackRecord(0)))

    val failure = assertFailsWith<IllegalArgumentException> { FeedbackRecords(paths).active() }

    assertEquals("Feedback record $file holds record f-00", failure.message)
  }
}
