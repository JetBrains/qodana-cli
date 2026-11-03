// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.support

import kotlinx.serialization.EncodeDefault
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.jsonObject
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
  Recording(exchanges.toList(), JsonNull, { GitHubReviewApi(KOTLIN, it, KOTLIN_BOTS) }, scripted = true)

/** A call of the Space client on [ULTIMATE] that sends exactly [exchanges] and has no recorded result. */
internal fun ultimateScripted(vararg exchanges: Exchange): Recording =
  Recording(exchanges.toList(), JsonNull, { SpaceReviewApi(ULTIMATE, it, emptySet()) }, scripted = true)

/**
 * One call of a provider client: the requests it sends, in order, the provider's answers, and the [result] the client
 * returned. Recordings come from the real repositories, with writes answered by script. A [scripted] one holds answers
 * no real provider gave: it stands for a repository state the real repositories don't have, from its first scripted
 * answer on.
 */
internal class Recording(
  val exchanges: List<Exchange>,
  val result: JsonElement,
  private val client: (CiHttpTransport) -> ReviewApi,
  private val scripted: Boolean = false,
) {
  inline fun <reified T> result(): T = wireJson.decodeFromJsonElement(result)

  /**
   * Runs [call] on a client whose requests are answered from this recording: each request must equal the next recorded
   * one and gets its recorded answer. With a real [provider], reads go to it instead, and must be the recorded reads.
   * Afterwards every recorded request must have been sent, whether [call] returned or failed.
   */
  fun <T> replay(provider: CiHttpTransport? = null, call: ReviewApi.() -> T): T {
    // A real provider answers as the real repository is, not as the script has it.
    require(provider == null || !scripted) { "A recording with scripted answers replays only from the recording" }
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
    if (provider != null && exchanges.map(Exchange::request).filter { it.method == "GET" } != reads) {
      throw AssertionError("The provider was read other than recorded: $reads", outcome.exceptionOrNull())
    }
    return outcome.getOrThrow()
  }

  /**
   * This call with the provider answering its first [method] request whose endpoint ends with [endpointEnd] with
   * [answer], after which the client sends nothing more.
   */
  fun failingAt(method: String, endpointEnd: String, answer: CiHttpResponse): Recording {
    val index = indexOf(method, endpointEnd)
    return Recording(exchanges.take(index) + Exchange(exchanges[index].request, answer), JsonNull, client, scripted = true)
  }

  /**
   * This call with its first [method] request whose endpoint ends with [endpointEnd] and whose query holds [query]
   * answered as [answers] makes of the recorded exchange: that exchange changed, and any further pages the change
   * makes the client read.
   */
  fun answering(
    method: String,
    endpointEnd: String,
    query: Map<String, String> = emptyMap(),
    answers: (Exchange) -> List<Exchange>,
  ): Recording {
    val index = indexOf(method, endpointEnd, query)
    return Recording(exchanges.take(index) + answers(exchanges[index]) + exchanges.drop(index + 1), JsonNull, client, scripted = true)
  }

  private fun indexOf(method: String, endpointEnd: String, query: Map<String, String> = emptyMap()): Int =
    exchanges.indexOfFirst {
      it.request.method == method && it.request.endpoint.endsWith(endpointEnd) && it.request.query.entries.containsAll(query.entries)
    }.also { require(it >= 0) { "The recording sends no $method …$endpointEnd with $query" } }
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

/** This JSON object with [entries] added or replaced. */
internal fun JsonElement.with(vararg entries: Pair<String, JsonElement>): JsonObject = JsonObject(jsonObject + entries)

/** This exchange with the provider's answer changed to [body], with the same status. */
internal fun Exchange.withBody(body: JsonElement): Exchange = copy(response = response.copy(body = body))

/** The exchange in which the provider answers this request with [response]. */
internal infix fun ProviderRequest.answeredWith(response: CiHttpResponse): Exchange = Exchange(this, response)

/** A provider answer with [status] and the JSON [body]. */
internal fun answer(status: Int, body: String): CiHttpResponse = CiHttpResponse(status, wireJson.parseToJsonElement(body))

/** The text of a provider recording, by its path under the `reviews` test resources. */
internal fun recording(path: String): String = resourceText("reviews/$path")

private fun stored(path: String, client: (CiHttpTransport) -> ReviewApi): Recording {
  val stored = wireJson.decodeFromString<StoredRecording>(recording("$path.json"))
  return Recording(
    stored.exchanges.map { Exchange(it.request, it.response.answer()) }, stored.result, client, scripted = stored.exchanges.any(StoredExchange::scripted),
  )
}

@Serializable
internal data class StoredRecording(val exchanges: List<StoredExchange>, val result: JsonElement)

/** A recorded exchange; a [scripted] one has an answer the recorder gave instead of the real provider. */
@OptIn(ExperimentalSerializationApi::class)
@Serializable
internal data class StoredExchange(
  val request: ProviderRequest,
  val response: StoredResponse,
  @EncodeDefault(EncodeDefault.Mode.NEVER) val scripted: Boolean = false,
)

@Serializable
internal data class StoredResponse(val status: Int, val body: JsonElement = JsonNull) {
  fun answer(): CiHttpResponse = CiHttpResponse(status, body)
}
