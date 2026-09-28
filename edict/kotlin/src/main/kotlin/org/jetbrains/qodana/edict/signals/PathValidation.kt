package org.jetbrains.qodana.edict.signals

private val revisionPattern = Regex("(?:[0-9a-f]{40}|[0-9a-f]{64})")

fun validRevision(value: String): Boolean = revisionPattern.matches(value)

fun validSourcePath(value: String): Boolean = value.isNotEmpty() && !value.startsWith('/') &&
  value.none { it in "\\:\u0000\r\n" } && value.split('/').none { it.isEmpty() || it == "." || it == ".." }
