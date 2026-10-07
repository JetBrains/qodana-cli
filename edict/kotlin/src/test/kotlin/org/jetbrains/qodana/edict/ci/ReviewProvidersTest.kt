package org.jetbrains.qodana.edict.ci

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.github.GitHubReviewApi
import org.jetbrains.qodana.edict.ci.space.SpaceReviewApi
import org.jetbrains.qodana.edict.edictnext.sha256Hex
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class ReviewProvidersTest {
  @Test
  fun `github creates branch file review and reviewer and reads review state`() {
    val transport = RecordingTransport { provider, method, endpoint, query, body ->
      assertEquals(CiProviderId.GITHUB, provider)
      when {
        method == "GET" && endpoint == "/repos/owner/repo" -> response(200, obj("default_branch" to JsonPrimitive("main")))
        method == "GET" && endpoint.contains("git/ref/heads/edict") -> response(404)
        method == "GET" && endpoint.contains("git/ref/heads/main") -> response(200, obj("object" to buildJsonObject { put("sha", "base-sha") }))
        method == "POST" && endpoint.endsWith("/git/refs") -> response(201, body ?: JsonObject(emptyMap()))
        method == "GET" && endpoint.contains("/contents/") -> response(404)
        method == "PUT" && endpoint.contains("/contents/") -> response(201)
        method == "GET" && endpoint.endsWith("/pulls") && query["state"] == "all" -> CiHttpResponse(200, JsonArray(emptyList()))
        method == "POST" && endpoint.endsWith("/pulls") -> response(201, obj(
          "number" to JsonPrimitive(17), "html_url" to JsonPrimitive("https://github.test/pr/17"),
        ))
        method == "POST" && endpoint.endsWith("/requested_reviewers") -> response(201)
        method == "GET" && endpoint.endsWith("/pulls/17") -> response(200, obj(
          "state" to JsonPrimitive("closed"), "merged_at" to JsonPrimitive(""),
        ))
        method == "GET" && endpoint.endsWith("/issues/17/comments") ->
          CiHttpResponse(200, JsonArray(listOf(obj("body" to JsonPrimitive("Please revise")))))
        method == "GET" && endpoint.endsWith("/pulls/17/reviews") -> CiHttpResponse(200, JsonArray(emptyList()))
        else -> error("Unexpected GitHub call $method $endpoint $query")
      }
    }
    val api = GitHubReviewApi(transport)
    val request = request(CiProviderId.GITHUB)
    assertEquals("main", api.resolveTargetBranch(request.repository, null))
    val pullRequest = api.ensureReview(request)
    assertEquals("17", pullRequest.id)
    assertEquals("https://github.test/pr/17", pullRequest.url)
    val reviewer = transport.calls.single { it.method == "POST" && it.endpoint.endsWith("/requested_reviewers") }
    assertEquals("reviewer", reviewer.body!!.getValue("reviewers").jsonArray.single().toString().trim('"'))
    val file = transport.calls.single { it.method == "PUT" }
    assertEquals("inspections/sample.inspection.kts", file.endpoint.substringAfter("/contents/"))

    val review = api.review(ReviewReference(CiProviderId.GITHUB, "owner", "repo", "17", pullRequest.url))
    assertEquals(ReviewState.CLOSED_UNMERGED, review.state)
    assertEquals("Please revise", review.evidence)
  }

  @Test
  fun `space creates commit review and reviewer and reads merged state`() {
    val transport = RecordingTransport { provider, method, endpoint, query, body ->
      assertEquals(CiProviderId.SPACE, provider)
      when {
        method == "GET" && endpoint == "/projects/key:owner/repositories/repo" -> response(
          200, obj("defaultBranch" to JsonPrimitive("trunk")),
        )
        method == "POST" && endpoint.endsWith("/commits") -> response(201)
        method == "GET" && endpoint.endsWith("/code-reviews") && query["branch"] != null ->
          CiHttpResponse(200, JsonArray(emptyList()))
        method == "POST" && endpoint.endsWith("/code-reviews") -> response(201, obj(
          "id" to JsonPrimitive("review-id"), "url" to JsonPrimitive("https://space.test/review-id"),
        ))
        method == "POST" && endpoint.endsWith("/reviewers") -> response(201, body ?: JsonObject(emptyMap()))
        method == "GET" && endpoint.endsWith("/review-id") -> response(200, obj("state" to JsonPrimitive("Merged")))
        else -> error("Unexpected Space call $method $endpoint $query")
      }
    }
    val api = SpaceReviewApi(transport, "https://space.test")
    val request = request(CiProviderId.SPACE)
    assertEquals("trunk", api.resolveTargetBranch(request.repository, null))
    val pullRequest = api.ensureReview(request)
    assertEquals("review-id", pullRequest.id)
    assertTrue(transport.calls.any {
      it.method == "POST" && it.endpoint.endsWith("/reviewers") && it.body?.get("reviewer") == JsonPrimitive("reviewer")
    })
    assertEquals(
      ReviewState.MERGED,
      api.review(ReviewReference(CiProviderId.SPACE, "owner", "repo", "review-id", pullRequest.url)).state,
    )
  }

  private fun request(provider: CiProviderId): CreateReviewRequest {
    val content = "inspection".encodeToByteArray()
    return CreateReviewRequest(
      "p-${"a".repeat(24)}",
      "sample",
      ReviewRepository(provider, "owner", "repo"),
      "main",
      "reviewer",
      "inspections/sample.inspection.kts",
      sha256Hex(content),
      content,
    )
  }

  private data class Call(
    val method: String,
    val endpoint: String,
    val query: Map<String, String>,
    val body: JsonObject?,
  )

  private class RecordingTransport(
    private val responder: (CiProviderId, String, String, Map<String, String>, JsonObject?) -> CiHttpResponse,
  ) : CiHttpTransport {
    val calls = mutableListOf<Call>()

    override fun send(
      provider: CiProviderId,
      method: String,
      endpoint: String,
      query: Map<String, String>,
      body: JsonObject?,
      requireAuthentication: Boolean,
    ): CiHttpResponse {
      calls += Call(method, endpoint, query, body)
      return responder(provider, method, endpoint, query, body)
    }
  }

  private fun response(status: Int, body: JsonObject = JsonObject(emptyMap())) = CiHttpResponse(status, body)
  private fun obj(vararg entries: Pair<String, kotlinx.serialization.json.JsonElement>) = JsonObject(entries.toMap())
}
