// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.extraction.reviews

import org.jetbrains.qodana.edict.ci.ReviewFetchResult
import org.jetbrains.qodana.edict.edictnext.support.pullRequest
import org.jetbrains.qodana.edict.edictnext.support.thread
import kotlin.test.Test
import kotlin.test.assertEquals

internal class ValidThreadsTest {
  @Test
  fun `valid pull requests and the provider's problems are kept as they are`() {
    val fetched = ReviewFetchResult(
      listOf(pullRequest(7, thread("a", start = 1, end = 1), thread("b", start = null, end = null))),
      listOf("Space PR 8: missing"),
    )

    assertEquals(fetched, fetched.withValidThreads())
  }

  @Test
  fun `a pull request without an exact base revision is dropped`() {
    val fetched = ReviewFetchResult(listOf(pullRequest(7, thread("a"), base = "f550b76")))

    assertEquals(ReviewFetchResult(emptyList(), listOf("PR 7: lacks exact base/head revisions")), fetched.withValidThreads())
  }

  @Test
  fun `a pull request without an exact head revision is dropped`() {
    val fetched = ReviewFetchResult(listOf(pullRequest(7, thread("a"), head = "")))

    assertEquals(ReviewFetchResult(emptyList(), listOf("PR 7: lacks exact base/head revisions")), fetched.withValidThreads())
  }

  @Test
  fun `a thread on a path outside the repository is dropped from its pull request`() {
    val fetched = ReviewFetchResult(listOf(pullRequest(7, thread("a"), thread("b", path = "../A.kt"))))

    assertEquals(
      ReviewFetchResult(listOf(pullRequest(7, thread("a"))), listOf("PR 7 discussion b: invalid path, revision or anchors")),
      fetched.withValidThreads(),
    )
  }

  @Test
  fun `a thread without an exact revision is dropped from its pull request`() {
    val fetched = ReviewFetchResult(listOf(pullRequest(7, thread("a"), thread("b", revision = "144c084424"))))

    assertEquals(
      ReviewFetchResult(listOf(pullRequest(7, thread("a"))), listOf("PR 7 discussion b: invalid path, revision or anchors")),
      fetched.withValidThreads(),
    )
  }

  @Test
  fun `a thread anchored at one end only is dropped from its pull request`() {
    val fetched = ReviewFetchResult(listOf(pullRequest(7, thread("a"), thread("b", start = 25, end = null))))

    assertEquals(
      ReviewFetchResult(listOf(pullRequest(7, thread("a"))), listOf("PR 7 discussion b: invalid path, revision or anchors")),
      fetched.withValidThreads(),
    )
  }

  @Test
  fun `a thread whose anchor ends before it starts is dropped from its pull request`() {
    val fetched = ReviewFetchResult(listOf(pullRequest(7, thread("a"), thread("b", start = 31, end = 25))))

    assertEquals(
      ReviewFetchResult(listOf(pullRequest(7, thread("a"))), listOf("PR 7 discussion b: invalid path, revision or anchors")),
      fetched.withValidThreads(),
    )
  }

  @Test
  fun `a thread anchored at line zero is dropped from its pull request`() {
    val fetched = ReviewFetchResult(listOf(pullRequest(7, thread("a"), thread("b", start = 0, end = 0))))

    assertEquals(
      ReviewFetchResult(listOf(pullRequest(7, thread("a"))), listOf("PR 7 discussion b: invalid path, revision or anchors")),
      fetched.withValidThreads(),
    )
  }

  @Test
  fun `problems follow the provider's, dropped pull requests first, in fetch order`() {
    val fetched = ReviewFetchResult(
      listOf(
        pullRequest(7, thread("a", path = "/A.kt")),
        pullRequest(8, thread("a"), head = ""),
        pullRequest(9, thread("b", path = "/B.kt")),
        pullRequest(10, thread("a"), base = ""),
      ),
      listOf("GitHub PR 11: Not Found"),
    )

    assertEquals(
      ReviewFetchResult(
        listOf(pullRequest(7), pullRequest(9)),
        listOf(
          "GitHub PR 11: Not Found",
          "PR 8: lacks exact base/head revisions",
          "PR 10: lacks exact base/head revisions",
          "PR 7 discussion a: invalid path, revision or anchors",
          "PR 9 discussion b: invalid path, revision or anchors",
        ),
      ),
      fetched.withValidThreads(),
    )
  }
}
