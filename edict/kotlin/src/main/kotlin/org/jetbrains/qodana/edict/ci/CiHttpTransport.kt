// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci

import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import org.jetbrains.qodana.edict.common.wireJson
import java.net.URI
import java.net.URLEncoder
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.time.Duration

internal fun urlPart(value: String): String = URLEncoder.encode(value, Charsets.UTF_8).replace("+", "%20")

internal fun isBot(name: String, kind: String): Boolean = kind.equals("bot", true) || name.lowercase().let {
  it.endsWith("[bot]") || it in listOf("dependabot", "github-actions", "patronus", "space-automation", "jetbrains-bot")
}

internal data class CiHttpResponse(
  val status: Int,
  val body: JsonElement,
  val hasNext: Boolean = false,
)

internal interface CiHttpTransport {
  fun send(
    provider: CiProviderId,
    method: String,
    endpoint: String,
    query: Map<String, String> = emptyMap(),
    body: JsonObject? = null,
    requireAuthentication: Boolean = false,
  ): CiHttpResponse
}

internal class DefaultCiHttpTransport(
  githubUrl: String = System.getenv("EDICT_GITHUB_API_URL") ?: "https://api.github.com",
  spaceUrl: String = System.getenv("EDICT_SPACE_URL") ?: "https://jetbrains.team",
  private val githubToken: String = System.getenv("GITHUB_TOKEN") ?: System.getenv("GH_TOKEN").orEmpty(),
  private val spaceToken: String = System.getenv("SPACE_TOKEN").orEmpty(),
) : CiHttpTransport {
  private val origins = mapOf(
    CiProviderId.GITHUB to validateOrigin(githubUrl),
    CiProviderId.SPACE to validateOrigin(spaceUrl).trimEnd('/') + "/api/http",
  )
  private val client = HttpClient.newBuilder()
    .connectTimeout(Duration.ofSeconds(30))
    .followRedirects(HttpClient.Redirect.NEVER)
    .build()

  override fun send(
    provider: CiProviderId,
    method: String,
    endpoint: String,
    query: Map<String, String>,
    body: JsonObject?,
    requireAuthentication: Boolean,
  ): CiHttpResponse {
    require(endpoint.startsWith('/') && '?' !in endpoint && '#' !in endpoint) { "Invalid provider API endpoint" }
    val token = if (provider == CiProviderId.GITHUB) githubToken else spaceToken
    require(provider != CiProviderId.SPACE || token.isNotBlank()) {
      "Space access requires SPACE_TOKEN in server environment"
    }
    require(!requireAuthentication || token.isNotBlank()) {
      "${provider.name.lowercase().replaceFirstChar(Char::uppercase)} review changes require a provider token in the server environment"
    }
    val suffix = query.entries.joinToString("&") { "${urlPart(it.key)}=${urlPart(it.value)}" }
    val url = origins.getValue(provider) + endpoint + suffix.takeIf(String::isNotEmpty)?.let { "?$it" }.orEmpty()
    val builder = HttpRequest.newBuilder(URI(url))
      .timeout(Duration.ofSeconds(30))
      .header("Accept", "application/json")
      .header("User-Agent", "edict-mcp")
    if (token.isNotEmpty()) builder.header("Authorization", "Bearer $token")
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
    catch (_: Exception) {
      error("${provider.name.lowercase()} API request failed (connection or timeout)")
    }
    val text = response.body().use { input ->
      val bytes = input.readNBytes(MAX_RESPONSE_BYTES + 1)
      require(bytes.size <= MAX_RESPONSE_BYTES) { "Provider response exceeds 16 MiB; refusing incomplete evidence" }
      bytes.toString(Charsets.UTF_8)
    }
    val parsed = if (text.isBlank()) JsonNull else try {
      wireJson.parseToJsonElement(text)
    }
    catch (_: Exception) {
      error("Invalid ${provider.name.lowercase()} API JSON response (HTTP ${response.statusCode()})")
    }
    return CiHttpResponse(
      response.statusCode(),
      parsed,
      response.headers().firstValue("Link").orElse("").contains("rel=\"next\""),
    )
  }

  private fun validateOrigin(value: String): String {
    val uri = URI(value.trimEnd('/'))
    require(
      uri.scheme in listOf("https", "http") && uri.host != null && uri.userInfo == null &&
        uri.query == null && uri.fragment == null,
    ) { "Invalid provider API URL" }
    return uri.toString().trimEnd('/')
  }

  private companion object {
    const val MAX_RESPONSE_BYTES = 16 * 1024 * 1024
  }
}
