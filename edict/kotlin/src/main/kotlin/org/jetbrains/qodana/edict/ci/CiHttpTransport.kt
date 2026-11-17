// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci

import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.doubleOrNull
import org.jetbrains.qodana.edict.common.wireJson
import java.net.URI
import java.net.URLEncoder
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.time.Duration

internal fun urlPart(value: String): String = URLEncoder.encode(value, Charsets.UTF_8).replace("+", "%20")

internal data class CiHttpResponse(
  val status: Int,
  val body: JsonElement,
)

internal interface CiHttpTransport {
  fun send(
    method: String,
    endpoint: String,
    query: Map<String, String> = emptyMap(),
    body: JsonObject? = null,
  ): CiHttpResponse
}

/** Sends requests authenticated by [token] to the provider API at [apiUrl], named by [provider] in errors. */
internal class DefaultCiHttpTransport(
  private val provider: CiProviderId,
  private val apiUrl: String,
  private val token: String,
) : CiHttpTransport {
  private val client = HttpClient.newBuilder()
    .connectTimeout(Duration.ofSeconds(30))
    .followRedirects(HttpClient.Redirect.NEVER)
    .build()

  override fun send(
    method: String,
    endpoint: String,
    query: Map<String, String>,
    body: JsonObject?,
  ): CiHttpResponse {
    require(endpoint.startsWith('/') && '?' !in endpoint && '#' !in endpoint) { "Invalid provider API endpoint" }
    val suffix = query.entries.joinToString("&") { "${urlPart(it.key)}=${urlPart(it.value)}" }
    val url = apiUrl + endpoint + suffix.takeIf(String::isNotEmpty)?.let { "?$it" }.orEmpty()
    val builder = HttpRequest.newBuilder(URI(url))
      .timeout(Duration.ofSeconds(30))
      .header("Accept", "application/json")
      .header("User-Agent", "edict-mcp")
      .header("Authorization", "Bearer $token")
    if (body == null) builder.method(method, HttpRequest.BodyPublishers.noBody())
    else {
      builder.header("Content-Type", "application/json")
        .method(method, HttpRequest.BodyPublishers.ofString(wireJson.encodeToString(body)))
    }
    val response = try {
      client.send(builder.build(), HttpResponse.BodyHandlers.ofInputStream())
    }
    catch (error: InterruptedException) {
      Thread.currentThread().interrupt()
      throw error
    }
    catch (e: Exception) {
      throw IllegalStateException("${provider.name.lowercase()} API request failed (connection or timeout)", e)
    }
    val text = response.body().use { input ->
      val bytes = input.readNBytes(MAX_RESPONSE_BYTES + 1)
      require(bytes.size <= MAX_RESPONSE_BYTES) { "Provider response exceeds 16 MiB; refusing incomplete evidence" }
      bytes.toString(Charsets.UTF_8)
    }
    val parsed = if (text.isBlank()) JsonNull else try {
      // The parser takes a lone unquoted word, such as a plain-text "Forbidden", for a JSON literal.
      wireJson.parseToJsonElement(text).also { require(it.isJson()) }
    }
    catch (e: Exception) {
      throw IllegalStateException("Invalid ${provider.name.lowercase()} API JSON response (HTTP ${response.statusCode()})", e)
    }
    return CiHttpResponse(response.statusCode(), parsed)
  }

  private companion object {
    const val MAX_RESPONSE_BYTES = 16 * 1024 * 1024
  }
}

/** False for an unquoted literal that is no JSON number, boolean or null. */
private fun JsonElement.isJson(): Boolean =
  this !is JsonPrimitive || isString || this is JsonNull || booleanOrNull != null || doubleOrNull != null
