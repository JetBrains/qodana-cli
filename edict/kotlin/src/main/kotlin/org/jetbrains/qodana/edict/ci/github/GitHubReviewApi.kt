// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.github

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.CiHttpResponse
import org.jetbrains.qodana.edict.ci.CiHttpTransport
import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.CreateReviewRequest
import org.jetbrains.qodana.edict.ci.CreatedReview
import org.jetbrains.qodana.edict.ci.ProviderReviewApi
import org.jetbrains.qodana.edict.ci.PullRequest
import org.jetbrains.qodana.edict.ci.ReviewMessage
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.ReviewSelection
import org.jetbrains.qodana.edict.ci.ReviewState
import org.jetbrains.qodana.edict.ci.ReviewStatus
import org.jetbrains.qodana.edict.ci.ReviewThread
import org.jetbrains.qodana.edict.ci.isBot
import org.jetbrains.qodana.edict.ci.urlPart
import org.jetbrains.qodana.edict.common.number
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.sha256
import org.jetbrains.qodana.edict.common.text
import java.nio.file.NoSuchFileException
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneOffset
import java.util.Base64

internal class GitHubReviewApi(
  private val http: CiHttpTransport,
) : ProviderReviewApi {
  override val id: CiProviderId = CiProviderId.GITHUB

  override fun fetch(selection: ReviewSelection): List<PullRequest> {
    require(selection.provider == id)
    val root = repositoryEndpoint(selection.repositoryRef)
    val numbers = selection.prNumbers.toMutableList()
    if (numbers.isEmpty()) {
      val start = LocalDate.parse(selection.startDate).atStartOfDay().toInstant(ZoneOffset.UTC)
      val seen = mutableSetOf<Int>()
      for (page in 1..MAX_PAGES) {
        val response = get(
          "$root/pulls",
          mapOf(
            "state" to "closed",
            "sort" to "updated",
            "direction" to "desc",
            "per_page" to "$PAGE_SIZE",
            "page" to "$page",
          ),
        )
        val items = response.body.jsonArray.map { it.jsonObject }
        var old = false
        for (item in items) {
          val number = item.number("number").toInt()
          require(number > 0 && seen.add(number)) { "Invalid or repeated PR number" }
          if (Instant.parse(item.text("updated_at")).isBefore(start)) {
            old = true
            break
          }
          if (
            item.text("merged_at").isNotEmpty() &&
            selection.containsDate(Instant.parse(item.text("merged_at")).toEpochMilli())
          ) numbers += number
          if (numbers.size == selection.maxPrs) break
        }
        if (old || numbers.size == selection.maxPrs || !response.hasNext && items.size < PAGE_SIZE) break
        require(items.isNotEmpty() && page < MAX_PAGES) { "Incomplete GitHub PR discovery" }
      }
    }
    return numbers.mapNotNull { number -> fetchPullRequest(root, number, selection) }
  }

  private fun fetchPullRequest(root: String, number: Int, selection: ReviewSelection): PullRequest? {
    val endpoint = "$root/pulls/$number"
    val value = get(endpoint).body.jsonObject
    require(value.number("number").toInt() == number) { "GitHub returned wrong PR" }
    if (value.text("merged_at").isEmpty()) return null
    val merged = Instant.parse(value.text("merged_at")).toEpochMilli()
    if (!selection.containsDate(merged)) return null
    val pullRequest = PullRequest(
      number,
      value.text("html_url"),
      value.text("title"),
      value.text("body"),
      value.obj("base").text("sha"),
      value.obj("head").text("sha"),
      merged,
    )
    val comments = pages("$endpoint/comments")
    val reviews = pages("$endpoint/reviews")
    val byId = comments.associateBy { it.number("id") }
    require(byId.size == comments.size && byId.keys.all { it > 0 }) { "Invalid or duplicate comment IDs" }
    val threads = comments.groupBy { it.number("in_reply_to_id").takeIf { id -> id > 0 } ?: it.number("id") }
      .mapNotNull { (commentId, group) ->
        val anchor = byId[commentId] ?: error("Missing root comment $commentId")
        val line = anchor.number("original_line").takeIf { it > 0 } ?: anchor.number("line")
        val startLine = anchor.number("original_start_line").takeIf { it > 0 }
          ?: anchor.number("start_line").takeIf { it > 0 }
          ?: line
        val messages = reviews.filter {
          it.number("id") == anchor.number("pull_request_review_id") && it.text("body").isNotBlank()
        }
          .filterNot { isBot(it.obj("user").text("login"), it.obj("user").text("type")) }
          .map { ReviewMessage(it.obj("user").text("login"), it.text("body"), it.text("submitted_at")) } +
          group.sortedWith(compareBy({ it.text("created_at") }, { it.number("id") }))
            .filterNot { isBot(it.obj("user").text("login"), it.obj("user").text("type")) }
            .map { ReviewMessage(it.obj("user").text("login"), it.text("body"), it.text("created_at")) }
        if (messages.isEmpty()) null else ReviewThread(
          "github-$number-$commentId",
          "${pullRequest.url}#discussion_r$commentId",
          anchor.text("path"),
          anchor.text("original_commit_id"),
          startLine.toInt(),
          line.toInt(),
          messages,
        )
      }.sortedWith(compareBy({ it.messages.first().createdAt }, { it.threadId }))
    return pullRequest.copy(threads = threads)
  }

  override fun file(repository: ReviewRepository, revision: String, path: String): String {
    require(repository.provider == id)
    val response = http.send(
      id,
      "GET",
      "${repositoryEndpoint(repository)}/contents/${path.split('/').joinToString("/", transform = ::urlPart)}",
      mapOf("ref" to revision),
    )
    if (response.status == 404) throw NoSuchFileException(path)
    val value = expectObject(response, 200)
    require(value.text("type") == "file" && value.text("encoding") == "base64") {
      "GitHub did not return complete base64 file content"
    }
    val bytes = Base64.getDecoder().decode(value.text("content").replace("\n", "").replace("\r", ""))
    require(bytes.size.toLong() == value.number("size") && bytes.none { it == 0.toByte() }) {
      "Provider file content is invalid, binary or truncated"
    }
    return bytes.toString(Charsets.UTF_8)
  }

  override fun resolveTargetBranch(repository: ReviewRepository, configured: String?): String {
    require(repository.provider == id)
    if (configured != null) return configured
    return expectObject(http.send(id, "GET", repositoryEndpoint(repository), requireAuthentication = true), 200)
      .text("default_branch")
      .also { require(it.isNotBlank()) { "GitHub repository has no default branch" } }
  }

  override fun ensureReview(request: CreateReviewRequest): CreatedReview {
    require(request.repository.provider == id)
    val root = repositoryEndpoint(request.repository)
    val branch = "edict/${request.clusterId}-${request.inspectionDigest.take(12)}"
    val branchRef = http.send(id, "GET", "$root/git/ref/heads/${urlPart(branch)}", requireAuthentication = true)
    if (branchRef.status == 404) {
      val base = expectObject(
        http.send(id, "GET", "$root/git/ref/heads/${urlPart(request.targetBranch)}", requireAuthentication = true),
        200,
      )
      val sha = base["object"]?.jsonObject?.text("sha").orEmpty()
      require(sha.isNotBlank()) { "GitHub target branch has no head revision" }
      val created = http.send(id, "POST", "$root/git/refs", body = buildJsonObject {
        put("ref", "refs/heads/$branch")
        put("sha", sha)
      }, requireAuthentication = true)
      require(created.status in setOf(201, 422)) { "GitHub branch creation returned HTTP ${created.status}" }
    }
    else require(branchRef.status == 200) { "GitHub branch lookup returned HTTP ${branchRef.status}" }

    val contentEndpoint = "$root/contents/${request.targetPath.split('/').joinToString("/", transform = ::urlPart)}"
    val current = http.send(id, "GET", contentEndpoint, mapOf("ref" to branch), requireAuthentication = true)
    var currentSha = ""
    var currentDigest = ""
    if (current.status == 200) {
      val value = current.body.jsonObject
      currentSha = value.text("sha")
      val encoded = value.text("content").replace("\n", "").replace("\r", "")
      if (encoded.isNotBlank()) currentDigest = sha256(Base64.getDecoder().decode(encoded))
    }
    else require(current.status == 404) { "GitHub target file lookup returned HTTP ${current.status}" }
    if (currentDigest != request.inspectionDigest) {
      val response = http.send(id, "PUT", contentEndpoint, body = buildJsonObject {
        put("message", "Add Edict inspection ${request.clusterId}")
        put("content", Base64.getEncoder().encodeToString(request.inspectionContent))
        put("branch", branch)
        if (currentSha.isNotBlank()) put("sha", currentSha)
      }, requireAuthentication = true)
      require(response.status in setOf(200, 201)) { "GitHub inspection commit returned HTTP ${response.status}" }
    }

    val found = http.send(
      id,
      "GET",
      "$root/pulls",
      mapOf("state" to "all", "head" to "${request.repository.owner}:$branch", "base" to request.targetBranch),
      requireAuthentication = true,
    )
    require(found.status == 200) { "GitHub pull-request lookup returned HTTP ${found.status}" }
    val existing = found.body.jsonArray.firstOrNull()?.jsonObject
    val pullRequest = existing ?: expectObject(http.send(id, "POST", "$root/pulls", body = buildJsonObject {
      put("title", "Add Edict inspection ${request.clusterId}")
      put("head", branch)
      put("base", request.targetBranch)
      put("body", "Generated Edict inspection version `${request.inspectionDigest}`.")
    }, requireAuthentication = true), 201)
    val number = pullRequest["number"]?.jsonPrimitive?.content.orEmpty()
    require(number.toLongOrNull()?.let { it > 0 } == true) { "GitHub pull request has no number" }
    val reviewer = http.send(id, "POST", "$root/pulls/$number/requested_reviewers", body = buildJsonObject {
      put("reviewers", JsonArray(listOf(JsonPrimitive(request.reviewer))))
    }, requireAuthentication = true)
    require(reviewer.status in setOf(200, 201, 422)) { "GitHub reviewer assignment returned HTTP ${reviewer.status}" }
    return CreatedReview(number, pullRequest.text("html_url"))
  }

  override fun review(reference: ReviewReference): ReviewStatus {
    require(reference.provider == id)
    val root = "/repos/${urlPart(reference.owner)}/${urlPart(reference.repository)}"
    val pull = expectObject(http.send(id, "GET", "$root/pulls/${urlPart(reference.id)}", requireAuthentication = true), 200)
    if (pull.text("merged_at").isNotBlank()) return ReviewStatus(ReviewState.MERGED)
    if (pull.text("state") == "open") return ReviewStatus(ReviewState.OPEN)
    val messages = listOf("$root/issues/${urlPart(reference.id)}/comments", "$root/pulls/${urlPart(reference.id)}/reviews")
      .flatMap(::pages)
      .mapNotNull { it.text("body").takeIf(String::isNotBlank) }
    return ReviewStatus(ReviewState.CLOSED_UNMERGED, messages.joinToString("\n\n"))
  }

  private fun pages(endpoint: String): List<JsonObject> {
    val all = mutableListOf<JsonObject>()
    for (page in 1..MAX_PAGES) {
      val response = get(endpoint, mapOf("per_page" to "$PAGE_SIZE", "page" to "$page"))
      val items = response.body.jsonArray.map { it.jsonObject }
      all += items
      if (!response.hasNext && items.size < PAGE_SIZE) return all
      require(items.isNotEmpty()) { "GitHub pagination advertised another page without items" }
    }
    error("GitHub pagination exceeded $MAX_PAGES pages; refusing incomplete evidence")
  }

  private fun get(endpoint: String, query: Map<String, String> = emptyMap()): CiHttpResponse =
    http.send(id, "GET", endpoint, query).also { response ->
      if (response.status == 404) throw NoSuchFileException("github source")
      check(response.status == 200) { "github API returned HTTP ${response.status}" }
    }

  private fun repositoryEndpoint(repository: ReviewRepository): String =
    "/repos/${urlPart(repository.owner)}/${urlPart(repository.repository)}"

  private fun expectObject(response: CiHttpResponse, vararg expected: Int): JsonObject {
    require(response.status in expected.toSet()) { "GitHub API returned HTTP ${response.status}" }
    return response.body as? JsonObject ?: error("GitHub API did not return an object")
  }

  private companion object {
    const val PAGE_SIZE = 100
    const val MAX_PAGES = 1000
  }
}
