// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.common

import kotlinx.serialization.json.*

val json = Json { encodeDefaults = true; prettyPrint = true; explicitNulls = false }
internal val wireJson = Json { encodeDefaults = true; explicitNulls = false }

internal fun JsonObject.text(key: String): String = (get(key) as? JsonPrimitive)?.contentOrNull.orEmpty()
internal fun JsonObject.number(key: String): Long = (get(key) as? JsonPrimitive)?.longOrNull ?: 0
internal fun JsonObject.flag(key: String): Boolean? = (get(key) as? JsonPrimitive)?.booleanOrNull
internal fun JsonObject.obj(key: String): JsonObject = get(key) as? JsonObject ?: JsonObject(emptyMap())
internal fun JsonObject.array(key: String): List<JsonObject> =
    (get(key) as? JsonArray)?.map { it.jsonObject } ?: emptyList()
