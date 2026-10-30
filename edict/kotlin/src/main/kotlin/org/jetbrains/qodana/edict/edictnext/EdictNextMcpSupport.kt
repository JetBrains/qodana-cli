package org.jetbrains.qodana.edict.edictnext

import io.modelcontextprotocol.kotlin.sdk.types.CallToolRequest
import io.modelcontextprotocol.kotlin.sdk.types.CallToolResult
import io.modelcontextprotocol.kotlin.sdk.types.TextContent
import io.modelcontextprotocol.kotlin.sdk.types.ToolSchema
import kotlinx.serialization.json.*

internal fun stringArguments(vararg arguments: Pair<String, String>): ToolSchema = ToolSchema(
  properties = buildJsonObject {
    arguments.forEach { (name, description) ->
      putJsonObject(name) {
        put("type", "string")
        put("description", description)
      }
    }
  },
  required = arguments.map(Pair<String, String>::first),
)

internal fun property(type: String, description: String): JsonObject = buildJsonObject {
  put("type", type)
  put("description", description)
}

internal fun toolSchema(required: List<String>, vararg properties: Pair<String, JsonObject>): ToolSchema = ToolSchema(
  properties = JsonObject(properties.toMap()),
  required = required,
)

internal fun CallToolRequest.requireString(name: String): String =
  arguments?.get(name)?.jsonPrimitive?.content
  ?: error("'$name' is required")

internal fun CallToolRequest.optionalString(name: String): String? = arguments?.get(name)?.jsonPrimitive?.contentOrNull

internal fun CallToolRequest.optionalInt(name: String): Int? =
  arguments?.get(name)?.jsonPrimitive?.let { it.intOrNull ?: error("'$name' must be an integer") }

internal inline fun <reified T> T.toToolResult(isError: Boolean = false): CallToolResult {
  val element = EdictNextJson.encodeToJsonElement(this)
  return CallToolResult(
    content = listOf(TextContent(EdictNextJson.encodeToString(this))),
    isError = isError.takeIf { it },
    structuredContent = element as? JsonObject,
  )
}
