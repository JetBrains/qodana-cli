package org.jetbrains.qodana.edict.git

import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.edictnext.EdictNextLineRange
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.signals.SignalValidation
import org.jetbrains.qodana.edict.support.fixtureSignals
import org.jetbrains.qodana.edict.support.gitFixture
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class CommitSignalExtractorTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `commit revision is the only persisted Git evidence and repository derives the rest`() {
    val repository = gitFixture(directory)
    val signals = fixtureSignals(repository)
    val revision = repository.resolve("HEAD")

    signals.forEach { signal ->
      assertEquals(EdictNextSignalSource.FromCommit(revision), signal.source)
      val content = json.encodeToString(signal)
      val sourceKeys = json.parseToJsonElement(content).jsonObject.getValue("source").jsonObject.keys
      assertEquals(setOf("type", "commitRevision"), sourceKeys)
      assertEquals(signal, SignalValidation.validate("inbox/${signal.id}.json", content))
      repository.validateEvidence(signal)
    }

    val invalid = signals.first().copy(
      fileRevision = signals.first().fileRevision.copy(expectedRanges = listOf(EdictNextLineRange(1, 1))),
    )
    assertFailsWith<IllegalArgumentException> { repository.validateEvidence(invalid) }
  }
}
