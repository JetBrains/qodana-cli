// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci

import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.transport.support.MAX_RESPONSE_BYTES
import org.jetbrains.qodana.edict.ci.transport.support.ProviderApiServer
import org.jetbrains.qodana.edict.ci.transport.support.ReceivedRequest
import org.jetbrains.qodana.edict.ci.transport.support.jsonTextOfSize
import org.jetbrains.qodana.edict.ci.transport.support.unreachableTransport
import org.junit.jupiter.api.extension.RegisterExtension
import org.junit.jupiter.params.ParameterizedTest
import org.junit.jupiter.params.provider.ValueSource
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

/**
 * What the transport does that the integration tests against the real providers can't show: request bodies (writes are
 * scripted there), empty and broken responses, the size cap, redirects, connection failures and interruption.
 */
internal class DefaultCiHttpTransportTest {
  @JvmField
  @RegisterExtension
  val api = ProviderApiServer()

  @Test
  fun `a request goes to the endpoint under the API URL with the token and JSON accepted`() {
    api.transport().send("GET", "/repos/JetBrains/kotlin")

    assertEquals(listOf(ReceivedRequest("GET", "/api/v3/repos/JetBrains/kotlin", GET_HEADERS, "")), api.received)
  }

  @Test
  fun `a JSON body is sent compactly with its content type`() {
    api.transport().send("PATCH", "/repos/JetBrains/kotlin/pulls/6007", body = buildJsonObject { put("state", "closed") })

    assertEquals(
      listOf(ReceivedRequest("PATCH", "/api/v3/repos/JetBrains/kotlin/pulls/6007", JSON_BODY_HEADERS, """{"state":"closed"}""")),
      api.received,
    )
  }

  @Test
  fun `a response without a body is JSON null`() {
    api.answer(204)

    assertEquals(CiHttpResponse(204, JsonNull), api.transport().send("DELETE", "/repos/JetBrains/kotlin/git/refs/heads/edict"))
  }

  @Test
  fun `a redirect is returned as it is, not followed`() {
    api.answer(301, headers = mapOf("Location" to "https://api.github.com/repositories/3432266"))

    assertEquals(CiHttpResponse(301, JsonNull), api.transport().send("GET", "/repos/JetBrains/kotlin"))
    assertEquals(listOf("/api/v3/repos/JetBrains/kotlin"), api.received.map(ReceivedRequest::target))
  }

  @Test
  fun `a truncated JSON response fails naming the provider and status`() {
    api.answer(200, """{"number":""")

    val error = assertFailsWith<IllegalStateException> { api.transport().send("GET", "/repos/JetBrains/kotlin/pulls/6007") }
    assertEquals("Invalid github API JSON response (HTTP 200)", error.message)
  }

  @Test
  fun `a plain-text response that is one word fails as invalid JSON`() {
    api.answer(403, "Forbidden")

    val error = assertFailsWith<IllegalStateException> { api.transport(CiProviderId.SPACE).send("GET", "/projects/key:ij") }
    assertEquals("Invalid space API JSON response (HTTP 403)", error.message)
  }

  @Test
  fun `a response of exactly 16 MiB is read`() {
    api.answer(200, jsonTextOfSize(MAX_RESPONSE_BYTES))

    assertEquals(
      CiHttpResponse(200, JsonPrimitive("x".repeat(MAX_RESPONSE_BYTES - 2))),
      api.transport().send("GET", "/repos/JetBrains/kotlin/contents/README.md"),
    )
  }

  @Test
  fun `a response over 16 MiB is refused`() {
    api.answer(200, jsonTextOfSize(MAX_RESPONSE_BYTES + 1))

    val error = assertFailsWith<IllegalArgumentException> { api.transport().send("GET", "/repos/JetBrains/kotlin/contents/README.md") }
    assertEquals("Provider response exceeds 16 MiB; refusing incomplete evidence", error.message)
  }

  @Test
  fun `a refused connection fails naming the provider`() {
    val error = assertFailsWith<IllegalStateException> { unreachableTransport(CiProviderId.SPACE).send("GET", "/projects/key:ij") }

    assertEquals("space API request failed (connection or timeout)", error.message)
  }

  @Test
  fun `an interrupted request is rethrown with the thread still interrupted`() {
    Thread.currentThread().interrupt()

    assertFailsWith<InterruptedException> { api.transport().send("GET", "/repos/JetBrains/kotlin") }
    assertTrue(Thread.interrupted())
  }

  @ParameterizedTest
  @ValueSource(strings = ["repos/JetBrains/kotlin", "/repos/JetBrains/kotlin?per_page=100", "/repos/JetBrains/kotlin#readme"])
  fun `an endpoint that is not an absolute path without query or fragment is refused before sending`(endpoint: String) {
    val error = assertFailsWith<IllegalArgumentException> { api.transport().send("GET", endpoint) }

    assertEquals("Invalid provider API endpoint", error.message)
    assertEquals(emptyList(), api.received)
  }
}

/** The headers of a request without a body. */
private val GET_HEADERS = mapOf(
  "Accept" to "application/json",
  "User-Agent" to "edict-mcp",
  "Authorization" to "Bearer provider-token",
  "Content-Type" to null,
)

/** The headers of a request with a JSON body. */
private val JSON_BODY_HEADERS = GET_HEADERS + ("Content-Type" to "application/json")
