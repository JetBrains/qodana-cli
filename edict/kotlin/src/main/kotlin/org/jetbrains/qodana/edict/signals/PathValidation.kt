package org.jetbrains.qodana.edict.signals

private val revisionPattern = Regex("[0-9a-f]{40}|[0-9a-f]{64}")

fun validRevision(value: String): Boolean = revisionPattern.matches(value)

private val digestPattern = Regex("[0-9a-f]{64}")

/** Whether [value] is the lowercase SHA-256 hex of some bytes, such as an inspection's. */
fun validDigest(value: String): Boolean = digestPattern.matches(value)

fun validSourcePath(value: String): Boolean = value.isNotEmpty() && !value.startsWith('/') &&
  value.none { it in "\\:\u0000\r\n" } && value.split('/').none { it.isEmpty() || it == "." || it == ".." }
