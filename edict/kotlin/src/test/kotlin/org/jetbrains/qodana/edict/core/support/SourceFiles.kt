// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.core.support

import org.jetbrains.qodana.edict.support.FakeReviewApi

/** A full commit SHA that the Git fixture doesn't have. */
internal const val MISSING_COMMIT = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

/** A review repository that has [content] as the file [path] at [revision], and no other file. */
internal fun reviewsHaving(revision: String, path: String, content: String): FakeReviewApi =
  FakeReviewApi().apply { files[revision to path] = content }
