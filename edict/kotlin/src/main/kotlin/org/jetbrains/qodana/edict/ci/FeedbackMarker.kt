// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci

private const val MARKER_START = "<!-- edict-feedback:"
private const val MARKER_END = " -->"

/** The marker that names [recordId] in its review comment, hidden from readers, so that an answer leads back to the record. */
internal fun feedbackMarker(recordId: String): String = "$MARKER_START$recordId$MARKER_END"

/** The record id that the marker in the review comment [body] names; null for a comment without one. */
internal fun feedbackRecordId(body: String): String? =
  body.substringAfterLast(MARKER_START, "").substringBefore(MARKER_END, "").ifEmpty { null }
