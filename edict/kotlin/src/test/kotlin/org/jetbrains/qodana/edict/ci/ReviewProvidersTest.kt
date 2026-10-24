package org.jetbrains.qodana.edict.ci

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.github.GitHubReviewApi
import org.jetbrains.qodana.edict.ci.space.SpaceReviewApi
import org.jetbrains.qodana.edict.edictnext.sha256Hex
import java.util.Base64
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

class ReviewProvidersTest {
  @Test
  fun `space skips malformed reviews and feed elements and reports every problem`() {
    val transport = RecordingTransport { _, method, endpoint, _, _ ->
      assertEquals("GET", method)
      when {
        endpoint.endsWith("/number:1") -> CiHttpResponse(200, JsonNull)
        endpoint.endsWith("/number:2") -> response(200, obj(
          "id" to JsonPrimitive("review-2"),
          "number" to JsonPrimitive(2),
          "state" to JsonPrimitive("Closed"),
          "timestamp" to JsonPrimitive(1),
          "title" to JsonPrimitive("Valid review"),
          "description" to JsonPrimitive(""),
          "feedChannelId" to JsonPrimitive("feed-2"),
          "branchPair" to obj(
            "repository" to JsonPrimitive("repo"),
            "isMerged" to JsonPrimitive(true),
            "sourceBranchRef" to JsonPrimitive("source"),
            "targetBranchInfo" to obj("ref" to JsonPrimitive("target")),
          ),
        ))
        endpoint == "/chats/messages/sync-batch" -> response(200, obj(
          "data" to JsonArray(listOf(JsonNull)),
          "hasMore" to JsonPrimitive(false),
        ))
        else -> error("Unexpected Space call $method $endpoint")
      }
    }
    val result = SpaceReviewApi(transport, "https://space.test").fetch(
      ReviewSelection(CiProviderId.SPACE, "owner", "repo", 2, prNumbers = listOf(1, 2)),
    )

    assertEquals(listOf(2), result.pullRequests.map(PullRequest::number))
    assertEquals(2, result.problems.size)
    assertTrue(result.problems.any { it.startsWith("Space PR 1:") })
    assertTrue(result.problems.any { "data[0]: expected object" in it })
  }

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
    val created = transport.calls.single { it.method == "POST" && it.endpoint.endsWith("/pulls") }
    assertEquals(JsonPrimitive(request.description), created.body!!["body"])

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
          200, obj("defaultBranch" to obj("head" to JsonPrimitive("refs/heads/trunk"))),
        )
        method == "GET" && endpoint.endsWith("/branches/refs%2Fheads%2Fmain") -> response(
          200, obj("branchInfo" to obj("ref" to JsonPrimitive("base-sha"))),
        )
        method == "GET" && endpoint.contains("/branches/") -> response(404)
        method == "POST" && endpoint.endsWith("/head") -> response(200)
        method == "POST" && endpoint.endsWith("/commit") -> response(200, obj("success" to JsonPrimitive(true)))
        method == "GET" && endpoint.endsWith("/code-reviews") && query["sourceBranch"] != null ->
          response(200, obj("data" to JsonArray(emptyList())))
        method == "POST" && endpoint.endsWith("/merge-requests") -> response(200, obj(
          "id" to JsonPrimitive("review-id"), "number" to JsonPrimitive(17),
        ))
        method == "POST" && endpoint.endsWith("/participants/reviewer") -> response(200)
        method == "GET" && endpoint.endsWith("/review-id") -> response(200, obj("state" to JsonPrimitive("Merged")))
        else -> error("Unexpected Space call $method $endpoint $query")
      }
    }
    val api = SpaceReviewApi(transport, "https://space.test")
    val request = request(CiProviderId.SPACE)
    assertEquals("refs/heads/trunk", api.resolveTargetBranch(request.repository, null))
    val pullRequest = api.ensureReview(request)
    assertEquals("review-id", pullRequest.id)
    assertEquals("https://space.test/p/owner/reviews/17", pullRequest.url)
    assertTrue(transport.calls.any {
      it.method == "POST" && it.endpoint.endsWith("/participants/reviewer") && it.body?.get("role") == JsonPrimitive("Reviewer")
    })
    val branch = transport.calls.single { it.method == "POST" && it.endpoint.endsWith("/head") }.body!!
    assertEquals(JsonPrimitive("refs/heads/edict/sample-${request.inspectionDigest.take(12)}"), branch["head"])
    assertEquals(JsonPrimitive("base-sha"), branch["target"])
    val commitCall = transport.calls.single { it.method == "POST" && it.endpoint.endsWith("/commit") }
    val created = transport.calls.single { it.method == "POST" && it.endpoint.endsWith("/merge-requests") }
    assertEquals(JsonPrimitive(request.description), created.body!!["description"])
    assertEquals("/projects/owner/repositories/repo/commit", commitCall.endpoint)
    val commit = commitCall.body!!
    assertEquals(JsonPrimitive("base-sha"), commit["baseCommit"])
    val file = commit.getValue("files").jsonArray.single() as JsonObject
    assertEquals(JsonPrimitive(request.targetPath), file["path"])
    assertEquals(obj(
      "className" to JsonPrimitive("GitFileContent.Base64"),
      "value" to JsonPrimitive(Base64.getEncoder().encodeToString(request.inspectionContent)),
    ), file["content"])
    assertEquals(
      ReviewState.MERGED,
      api.review(ReviewReference(CiProviderId.SPACE, "owner", "repo", "review-id", pullRequest.url)).state,
    )
  }

  @Test
  fun `space reuses the exact branch and wrapped review without committing again`() {
    val request = request(CiProviderId.SPACE)
    val branch = "edict/sample-${request.inspectionDigest.take(12)}"
    fun review(target: String, id: String) = obj("review" to obj(
      "id" to JsonPrimitive(id),
      "number" to JsonPrimitive(17),
      "branchPair" to obj(
        "repository" to JsonPrimitive("repo"),
        "sourceBranch" to JsonPrimitive("refs/heads/$branch"),
        "targetBranch" to JsonPrimitive(target),
      ),
    ))
    val transport = RecordingTransport { _, method, endpoint, query, _ ->
      when {
        method == "GET" && endpoint.contains("/branches/") -> response(
          200, obj("branchInfo" to obj("ref" to JsonPrimitive("existing-sha"))),
        )
        method == "GET" && endpoint.endsWith("/files") -> CiHttpResponse(200, JsonArray(listOf(obj(
          "path" to JsonPrimitive(request.targetPath), "type" to JsonPrimitive("FILE"), "blob" to JsonPrimitive("blob"),
        ))))
        method == "GET" && endpoint.endsWith("/content") -> response(200, obj(
          "partBase64" to JsonPrimitive(Base64.getEncoder().encodeToString(request.inspectionContent)),
          "totalSize" to JsonPrimitive(request.inspectionContent.size),
        ))
        method == "GET" && endpoint.endsWith("/code-reviews") -> {
          assertEquals(branch, query["sourceBranch"])
          assertEquals("main", query["targetBranch"])
          response(200, obj("data" to JsonArray(listOf(review("other", "wrong"), review("main", "existing")))))
        }
        method == "POST" && endpoint.endsWith("/existing/participants/reviewer") -> response(404)
        method == "POST" && endpoint.endsWith("/existing/participants/username%3Areviewer") -> response(200)
        else -> error("Unexpected Space call $method $endpoint")
      }
    }
    val result = SpaceReviewApi(transport, "https://space.test").ensureReview(request)
    assertEquals("existing", result.id)
    assertTrue(transport.calls.any { it.endpoint.endsWith("/participants/username%3Areviewer") })
    assertTrue(transport.calls.none { it.endpoint.endsWith("/commit") || it.endpoint.endsWith("/head") || it.endpoint.endsWith("/merge-requests") })
  }

  @Test
  fun `space stops when the commit operation reports failure inside HTTP 200`() {
    val transport = RecordingTransport { _, method, endpoint, _, _ ->
      when {
        method == "GET" && endpoint.endsWith("/branches/refs%2Fheads%2Fmain") -> response(
          200, obj("branchInfo" to obj("ref" to JsonPrimitive("base-sha"))),
        )
        method == "GET" && endpoint.contains("/branches/") -> response(404)
        method == "POST" && endpoint.endsWith("/head") -> response(200)
        method == "POST" && endpoint.endsWith("/commit") -> response(200, obj(
          "success" to JsonPrimitive(false), "message" to JsonPrimitive("Branch changed concurrently"),
        ))
        else -> error("Unexpected Space call $method $endpoint")
      }
    }
    val failure = assertFailsWith<IllegalArgumentException> {
      SpaceReviewApi(transport).ensureReview(request(CiProviderId.SPACE))
    }
    assertTrue(failure.message.orEmpty().contains("Branch changed concurrently"))
    assertTrue(transport.calls.none { it.endpoint.contains("/code-reviews") })
  }

  @Test
  fun `space resumes a branch left at the target revision and reuses a closed review`() {
    val request = request(CiProviderId.SPACE).copy(targetBranch = "refs/heads/main")
    val branch = "edict/sample-${request.inspectionDigest.take(12)}"
    val transport = RecordingTransport { _, method, endpoint, query, _ ->
      when {
        method == "GET" && endpoint.contains("/branches/") -> response(
          200, obj("branchInfo" to obj("ref" to JsonPrimitive("base-sha"))),
        )
        method == "GET" && endpoint.endsWith("/files") -> CiHttpResponse(200, JsonArray(emptyList()))
        method == "POST" && endpoint.endsWith("/commit") -> response(200, obj("success" to JsonPrimitive(true)))
        method == "GET" && endpoint.endsWith("/code-reviews") -> {
          assertEquals("main", query["targetBranch"])
          response(200, obj("data" to JsonArray(if (query["state"] == "Opened") emptyList() else listOf(obj(
            "review" to obj(
              "id" to JsonPrimitive("closed-review"),
              "number" to JsonPrimitive(18),
              "branchPair" to obj(
                "repository" to JsonPrimitive("repo"),
                "sourceBranch" to JsonPrimitive(branch),
                "targetBranch" to JsonPrimitive("refs/heads/main"),
              ),
            ),
          )))))
        }
        method == "POST" && endpoint.endsWith("/closed-review/participants/reviewer") -> response(200)
        else -> error("Unexpected Space call $method $endpoint")
      }
    }
    assertEquals("closed-review", SpaceReviewApi(transport).ensureReview(request).id)
    assertEquals(1, transport.calls.count { it.method == "POST" && it.endpoint.endsWith("/commit") })
    assertTrue(transport.calls.none { it.endpoint.endsWith("/head") || it.endpoint.endsWith("/merge-requests") })
  }

  @Test
  fun `space never overwrites an inspection branch with different content`() {
    val transport = RecordingTransport { _, method, endpoint, _, _ ->
      when {
        method == "GET" && endpoint.endsWith("/branches/refs%2Fheads%2Fmain") -> response(
          200, obj("branchInfo" to obj("ref" to JsonPrimitive("base-sha"))),
        )
        method == "GET" && endpoint.contains("/branches/") -> response(
          200, obj("branchInfo" to obj("ref" to JsonPrimitive("different-sha"))),
        )
        method == "GET" && endpoint.endsWith("/files") -> CiHttpResponse(200, JsonArray(emptyList()))
        else -> error("Unexpected Space call $method $endpoint")
      }
    }
    val failure = assertFailsWith<IllegalArgumentException> {
      SpaceReviewApi(transport).ensureReview(request(CiProviderId.SPACE))
    }
    assertTrue(failure.message.orEmpty().contains("already exists with different content"))
    assertTrue(transport.calls.all { it.method == "GET" })
  }

  @Test
  fun `space includes permission details and stops before committing`() {
    val transport = RecordingTransport { _, method, endpoint, _, _ ->
      when {
        method == "GET" && endpoint.endsWith("/branches/refs%2Fheads%2Fmain") -> response(
          200, obj("branchInfo" to obj("ref" to JsonPrimitive("base-sha"))),
        )
        method == "GET" && endpoint.contains("/branches/") -> CiHttpResponse(200, JsonNull)
        method == "POST" && endpoint.endsWith("/head") -> response(403, obj(
          "error_description" to JsonPrimitive("Permission is not granted: VcsRepository.Write"),
        ))
        else -> error("Unexpected Space call $method $endpoint")
      }
    }
    val failure = assertFailsWith<IllegalArgumentException> {
      SpaceReviewApi(transport).ensureReview(request(CiProviderId.SPACE))
    }
    assertEquals("Space inspection branch creation returned HTTP 403: Permission is not granted: VcsRepository.Write", failure.message)
    assertTrue(transport.calls.none { it.endpoint.endsWith("/commit") || it.endpoint.contains("/code-reviews") })
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
      "## Description\n\nDetects a recurring problem.\n\n## Strong signals\n\n" +
        (1..5).joinToString("\n") { "$it. STRONG: [PR #$it](https://example.com/pull/$it)" },
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
