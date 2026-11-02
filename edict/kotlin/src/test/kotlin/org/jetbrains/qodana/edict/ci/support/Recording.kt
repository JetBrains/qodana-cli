// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.support

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.decodeFromJsonElement
import org.jetbrains.qodana.edict.ci.CiHttpResponse
import org.jetbrains.qodana.edict.ci.CiHttpTransport
import org.jetbrains.qodana.edict.ci.ReviewApi
import org.jetbrains.qodana.edict.ci.github.GitHubReviewApi
import org.jetbrains.qodana.edict.ci.space.SpaceReviewApi
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.support.resourceText
import kotlin.test.assertEquals
import kotlin.test.fail

/**
 * A call of the GitHub client on [KOTLIN], stored as `reviews/github/kotlin/<name>.json` in the test resources, replayed
 * on a client that leaves out [bots].
 */
internal fun kotlinRecording(name: String, bots: Set<String> = KOTLIN_BOTS): Recording =
  stored("github/kotlin/$name") { GitHubReviewApi(KOTLIN, it, bots) }

/**
 * A call of the Space client on [ULTIMATE], stored as `reviews/space/ultimate/<name>.json` in the test resources,
 * replayed on a client that leaves out [bots].
 */
internal fun ultimateRecording(name: String, bots: Set<String> = emptySet()): Recording =
  stored("space/ultimate/$name") { SpaceReviewApi(ULTIMATE, it, bots) }

/** A call of the GitHub client on [KOTLIN] that sends exactly [exchanges] and has no recorded result. */
internal fun kotlinScripted(vararg exchanges: Exchange): Recording =
  Recording(exchanges.toList(), JsonNull) { GitHubReviewApi(KOTLIN, it, KOTLIN_BOTS) }

/** A call of the Space client on [ULTIMATE] that sends exactly [exchanges] and has no recorded result. */
internal fun ultimateScripted(vararg exchanges: Exchange): Recording =
  Recording(exchanges.toList(), JsonNull) { SpaceReviewApi(ULTIMATE, it, emptySet()) }

/**
 * One call of a provider client: the requests it sends, in order, the provider's answers, and the [result] the client
 * returned. Recordings come from the real repositories, with writes answered by script; scripted ones stand for
 * repository states the real repositories don't have.
 */
internal class Recording(
  val exchanges: List<Exchange>,
  val result: JsonElement,
  private val client: (CiHttpTransport) -> ReviewApi,
) {
  inline fun <reified T> result(): T = wireJson.decodeFromJsonElement(result)

  /**
   * Runs [call] on a client whose requests are answered from this recording: each request must equal the next recorded
   * one and gets its recorded answer. With a real [provider], reads go to it instead, and must be the recorded reads.
   * Afterwards every recorded request must have been sent, whether [call] returned or failed.
   */
  fun <T> replay(provider: CiHttpTransport? = null, call: ReviewApi.() -> T): T {
    val unsent = ArrayDeque(exchanges.filter { provider == null || it.request.method != "GET" })
    val reads = mutableListOf<ProviderRequest>()
    val transport = object : CiHttpTransport {
      override fun send(method: String, endpoint: String, query: Map<String, String>, body: JsonObject?): CiHttpResponse {
        val request = ProviderRequest(method, endpoint, query, body)
        if (provider != null && method == "GET") return provider.send(method, endpoint, query, body).also { reads += request }
        val next = unsent.removeFirstOrNull() ?: fail("Unrecorded request $request")
        assertEquals(next.request, request)
        return next.response
      }
    }
    val outcome = runCatching { client(transport).call() }
    if (unsent.isNotEmpty()) {
      throw AssertionError("Recorded requests were not sent: ${unsent.map(Exchange::request)}", outcome.exceptionOrNull())
    }
    if (provider != null) assertEquals(exchanges.map(Exchange::request).filter { it.method == "GET" }, reads)
    return outcome.getOrThrow()
  }

  /**
   * This call with the provider answering its first [method] request whose endpoint ends with [endpointEnd] with
   * [answer], after which the client sends nothing more.
   */
  fun failingAt(method: String, endpointEnd: String, answer: CiHttpResponse): Recording {
    val index = exchanges.indexOfFirst { it.request.method == method && it.request.endpoint.endsWith(endpointEnd) }
    require(index >= 0) { "The recording sends no $method …$endpointEnd" }
    return Recording(exchanges.take(index) + Exchange(exchanges[index].request, answer), JsonNull, client)
  }
}

/** A request to a provider API as a test expects it; equal requests have the same query and JSON body. */
@Serializable
internal data class ProviderRequest(
  val method: String,
  val endpoint: String,
  val query: Map<String, String> = emptyMap(),
  val body: JsonObject? = null,
)

/** A request a test expects, and the provider's answer to it. */
internal data class Exchange(val request: ProviderRequest, val response: CiHttpResponse)

/** The exchange in which the provider answers this request with [response]. */
internal infix fun ProviderRequest.answeredWith(response: CiHttpResponse): Exchange = Exchange(this, response)

/** A provider answer with [status] and the JSON [body]. */
internal fun answer(status: Int, body: String): CiHttpResponse = CiHttpResponse(status, wireJson.parseToJsonElement(body))

/** The text of a provider recording, by its path under the `reviews` test resources. */
internal fun recording(path: String): String = resourceText("reviews/$path")

private fun stored(path: String, client: (CiHttpTransport) -> ReviewApi): Recording {
  val stored = wireJson.decodeFromString<StoredRecording>(recording("$path.json"))
  return Recording(stored.exchanges.map { Exchange(it.request, it.response.answer()) }, stored.result, client)
}

@Serializable
internal data class StoredRecording(val exchanges: List<StoredExchange>, val result: JsonElement = JsonNull)

@Serializable
internal data class StoredExchange(val request: ProviderRequest, val response: StoredResponse)

@Serializable
internal data class StoredResponse(val status: Int, val body: JsonElement = JsonNull, val hasNext: Boolean = false) {
  fun answer(): CiHttpResponse = CiHttpResponse(status, body, hasNext)
}
