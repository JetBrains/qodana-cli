// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import org.jetbrains.qodana.edict.feedback.support.feedbackRecord
import org.junit.jupiter.api.Test
import kotlin.test.assertEquals

class FeedbackCommentTest {
  @Test
  fun `says it is a test finding, quotes the problem, asks for a reaction and folds the description away`() {
    assertEquals(
      """
        **Edict test finding.** This inspection was generated from the project's reviews and is not established yet.
        Your assessment decides whether it becomes one; this finding does not affect the analysis result.

        > Blocking call on the worker thread

        **Is this a real problem?** React with 👍 if it is, or with 👎 if it is not.
        A reply that explains why is welcome too: it is kept as context of your answer.

        <details>
        <summary>Worker thread blocking</summary>

        Reports blocking calls on the worker thread.

        </details>

        <!-- edict-feedback:f-00 -->
      """.trimIndent(),
      feedbackComment(feedbackRecord(0)),
    )
  }
}
