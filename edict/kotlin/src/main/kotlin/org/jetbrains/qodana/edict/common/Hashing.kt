// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.common

import java.security.MessageDigest
import java.security.SecureRandom

fun sha256(value: String): String =
    MessageDigest.getInstance("SHA-256").digest(value.toByteArray(Charsets.UTF_8)).toHexString()

internal fun randomId(): String = ByteArray(32).also(SecureRandom()::nextBytes).toHexString()
