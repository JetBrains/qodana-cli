// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.space

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.CiHttpResponse
import org.jetbrains.qodana.edict.ci.CiHttpTransport
import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.CreateReviewRequest
import org.jetbrains.qodana.edict.ci.CreatedReview
import org.jetbrains.qodana.edict.ci.ProviderReviewApi
import org.jetbrains.qodana.edict.ci.PullRequest
import org.jetbrains.qodana.edict.ci.ReviewMessage
import org.jetbrains.qodana.edict.ci.ReviewFetchResult
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.ReviewSelection
import org.jetbrains.qodana.edict.ci.ReviewState
import org.jetbrains.qodana.edict.ci.ReviewStatus
import org.jetbrains.qodana.edict.ci.ReviewThread
import org.jetbrains.qodana.edict.ci.isBot
import org.jetbrains.qodana.edict.ci.urlPart
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.number
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import java.nio.file.NoSuchFileException
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneOffset
import java.util.Base64

internal class SpaceReviewApi(
  private val http: CiHttpTransport,
  spaceUrl: String = System.getenv("EDICT_SPACE_URL") ?: "https://jetbrains.team",
) : ProviderReviewApi {
  override val id: CiProviderId = CiProviderId.SPACE
  private val webOrigin = spaceUrl.trimEnd('/')

  override fun fetch(selection: ReviewSelection): ReviewFetchResult {
    require(selection.provider == id)
    val root = "/projects/key:${urlPart(selection.owner)}/code-reviews"
    val numbers = selection.prNumbers.toMutableList()
    val problems = mutableListOf<String>()
    if (numbers.isEmpty()) {
      val start = LocalDate.parse(selection.startDate).atStartOfDay().toInstant(ZoneOffset.UTC).toEpochMilli()
      val seen = mutableSetOf<Int>()
      for (page in 0 until MAX_PAGES) {
        val response = get(
          root,
          mapOf(
            "\$skip" to "${page * PAGE_SIZE}",
            "\$top" to "$PAGE_SIZE",
            "state" to "Merged",
            "repository" to selection.repository,
            "sort" to "LastUpdatedDesc",
            "to" to selection.endDate,
            "\$fields" to "data(review(id,number,timestamp))",
          ),
        ).body as? JsonObject
        if (response == null) {
          problems += "Space review discovery page ${page + 1}: expected an object response"
          break
        }
        val data = response.objectElements("data", "Space review discovery page ${page + 1}", problems)
        var old = false
        for ((index, wrapper) in data.objects.withIndex()) {
          val item = wrapper["review"] as? JsonObject
          if (item == null) {
            problems += "Space review discovery page ${page + 1} data[$index]: missing review object"
            continue
          }
          val number = item.number("number").toInt()
          if (number <= 0 || !seen.add(number) || item.number("timestamp") <= 0) {
            problems += "Space review discovery page ${page + 1} data[$index]: invalid or repeated review"
            continue
          }
          if (item.number("timestamp") < start) {
            old = true
            break
          }
          if (selection.containsDate(item.number("timestamp"))) numbers += number
          if (numbers.size == selection.maxPrs) break
        }
        if (old || numbers.size == selection.maxPrs || data.elementCount < PAGE_SIZE) break
        if (page == MAX_PAGES - 1) problems += "Incomplete Space review discovery after $MAX_PAGES pages"
      }
    }
    val pullRequests = numbers.mapNotNull { number ->
      try {
        fetchPullRequest(root, number, selection, problems)
      }
      catch (e: Exception) {
        problems += "Space PR $number: ${e.message ?: e.javaClass.simpleName}"
        null
      }
    }
    return ReviewFetchResult(pullRequests, problems)
  }

  private fun fetchPullRequest(
    root: String,
    number: Int,
    selection: ReviewSelection,
    problems: MutableList<String>,
  ): PullRequest? {
    val value = get(
      "$root/number:$number",
      mapOf(
        "\$fields" to "id,number,state,timestamp,title,description,feedChannelId," +
          "branchPair(repository,isMerged,sourceBranchRef,targetBranchInfo(ref))",
      ),
    ).body.jsonObject
    require(value.number("number").toInt() == number && value.text("state").isNotEmpty()) {
      "Wrong Space review or missing merge state"
    }
    if (value.text("state") != "Closed") return null
    val pair = value.obj("branchPair")
    require(pair.text("repository") == selection.repository && pair.flag("isMerged") != null) {
      "Invalid Space review repository or merge status"
    }
    if (pair.flag("isMerged") != true || !selection.containsDate(value.number("timestamp"))) return null
    val feed = value.text("feedChannelId")
    require(feed.isNotEmpty()) { "Space review lacks discussion feed" }
    val threads = spaceFeed(feed, "Space PR $number feed", problems).mapIndexedNotNull { index, message ->
      try {
        val details = message.obj("details")
        val author = message.obj("projectedItem").obj("author")
        if (
          details.text("className") != "CodeDiscussionAddedFeedEvent" ||
          author.obj("details").obj("user").isEmpty() ||
          isBot(author.text("name"), author.obj("details").text("className"))
        ) return@mapIndexedNotNull null
        val discussion = details.obj("codeDiscussion")
        val channel = discussion.obj("channel").text("id")
        require(channel.isNotEmpty()) { "discussion lacks channel" }
        val messages = spaceDiscussion(channel, "Space PR $number discussion $channel", problems)
        if (messages.isEmpty()) return@mapIndexedNotNull null
        val anchor = discussion.obj("anchor")
        val line = (anchor.number("line").takeIf { it > 0 } ?: anchor.number("oldLine").takeIf { it > 0 })?.toInt()
        ReviewThread(
          "space-$number-${message.text("id")}",
          "$webOrigin/im/review/${urlPart(value.text("id"))}?message=${urlPart(message.text("id"))}&channel=${urlPart(feed)}",
          anchor.text("filename").removePrefix("/"),
          anchor.text("revision"),
          line,
          line,
          messages,
        )
      }
      catch (e: Exception) {
        problems += "Space PR $number feed item $index: ${e.message ?: e.javaClass.simpleName}"
        null
      }
    }.sortedWith(compareBy({ it.messages.first().createdAt }, { it.threadId }))
    return PullRequest(
      number,
      "$webOrigin/p/${urlPart(selection.owner)}/reviews/$number",
      value.text("title"),
      value.text("description"),
      pair.obj("targetBranchInfo").text("ref"),
      pair.text("sourceBranchRef"),
      value.number("timestamp"),
      threads,
    )
  }

  override fun file(repository: ReviewRepository, revision: String, path: String): String {
    require(repository.provider == id)
    val root = repositoryEndpoint(repository)
    val files = get(root + "/files", mapOf("commit" to revision, "path" to path)).body as? JsonArray
      ?: error("Space API did not return a file list")
    val file = files.map { it.jsonObject }
      .firstOrNull { it.text("path") == path && it.text("type") in listOf("FILE", "EXE_FILE") }
      ?: throw NoSuchFileException(path)
    val value = get(
      root + "/content",
      mapOf("blobId" to file.text("blob"), "skip" to "0", "limit" to "16777216"),
    ).body.jsonObject
    val bytes = Base64.getDecoder().decode(value.text("partBase64").replace("\n", "").replace("\r", ""))
    require(bytes.size.toLong() == value.number("totalSize") && bytes.none { it == 0.toByte() }) {
      "Provider file content is invalid, binary or truncated"
    }
    return bytes.toString(Charsets.UTF_8)
  }

  override fun resolveTargetBranch(repository: ReviewRepository, configured: String?): String {
    require(repository.provider == id)
    if (configured != null) return configured
    return expectObject(http.send(id, "GET", repositoryEndpoint(repository), requireAuthentication = true), 200)
      .text("defaultBranch")
      .also { require(it.isNotBlank()) { "Space repository has no default branch" } }
  }

  override fun ensureReview(request: CreateReviewRequest): CreatedReview {
    require(request.repository.provider == id)
    val repository = repositoryEndpoint(request.repository)
    val branch = "edict/${request.clusterId}-${request.inspectionDigest.take(12)}"
    val commit = http.send(id, "POST", "$repository/commits", body = buildJsonObject {
      put("branch", branch)
      put("startPoint", request.targetBranch)
      put("message", "Add Edict inspection ${request.clusterId}")
      put("changes", buildJsonArray {
        add(buildJsonObject {
          put("changeType", "ADD_OR_MODIFY")
          put("path", request.targetPath)
          put("contentBase64", Base64.getEncoder().encodeToString(request.inspectionContent))
        })
      })
    }, requireAuthentication = true)
    require(commit.status in setOf(200, 201, 409)) { "Space inspection commit returned HTTP ${commit.status}" }

    val reviews = http.send(
      id,
      "GET",
      "/projects/key:${urlPart(request.repository.owner)}/code-reviews",
      mapOf("repository" to request.repository.repository, "branch" to branch),
      requireAuthentication = true,
    )
    require(reviews.status == 200) { "Space review lookup returned HTTP ${reviews.status}" }
    val existing = (reviews.body as? JsonArray)?.firstOrNull()?.jsonObject
      ?: (reviews.body as? JsonObject)?.get("data")?.let { it as? JsonArray }?.firstOrNull()?.jsonObject
    val review = existing ?: expectObject(http.send(
      id,
      "POST",
      "/projects/key:${urlPart(request.repository.owner)}/code-reviews",
      body = buildJsonObject {
        put("title", "Add Edict inspection ${request.clusterId}")
        put("repository", request.repository.repository)
        put("sourceBranch", branch)
        put("targetBranch", request.targetBranch)
      },
      requireAuthentication = true,
    ), 200, 201)
    val reviewId = review.text("id").ifBlank { review.number("number").takeIf { it > 0 }?.toString().orEmpty() }
    require(reviewId.isNotBlank()) { "Space code review has no id" }
    val reviewer = http.send(
      id,
      "POST",
      "/projects/key:${urlPart(request.repository.owner)}/code-reviews/${urlPart(reviewId)}/reviewers",
      body = buildJsonObject { put("reviewer", request.reviewer) },
      requireAuthentication = true,
    )
    require(reviewer.status in setOf(200, 201, 409)) { "Space reviewer assignment returned HTTP ${reviewer.status}" }
    val url = review.text("url").ifBlank {
      review.text("reviewUrl").ifBlank { "$webOrigin/p/${request.repository.owner}/reviews/$reviewId" }
    }
    return CreatedReview(reviewId, url)
  }

  override fun review(reference: ReviewReference): ReviewStatus {
    require(reference.provider == id)
    val response = expectObject(
      http.send(
        id,
        "GET",
        "/projects/key:${urlPart(reference.owner)}/code-reviews/${urlPart(reference.id)}",
        query = mapOf("\$fields" to "state,reason,description,branchPair(isMerged)"),
        requireAuthentication = true,
      ),
      200,
    )
    return when {
      response.text("state").uppercase() in setOf("OPEN", "OPENED") -> ReviewStatus(ReviewState.OPEN)
      response.text("state").uppercase() == "MERGED" || response.obj("branchPair").flag("isMerged") == true ->
        ReviewStatus(ReviewState.MERGED)
      else -> ReviewStatus(
        ReviewState.CLOSED_UNMERGED,
        response.text("reason").ifBlank { response.text("description") },
      )
    }
  }

  private fun spaceFeed(channel: String, context: String, problems: MutableList<String>): List<JsonObject> {
    val all = linkedMapOf<String, JsonObject>()
    var etag = "0"
    val cursors = mutableSetOf(etag)
    for (page in 0 until MAX_PAGES) {
      val response = get(
        "/chats/messages/sync-batch",
        mapOf(
          "channel" to "id:$channel",
          "batchInfo" to "{etag:$etag,batchSize:$PAGE_SIZE}",
          "\$fields" to "data(chatMessage(id,text,created,details(className,codeDiscussion(id,channel(id)," +
            "anchor(filename,line,oldLine,revision))),projectedItem(author(name,details(className,user(id)))))),etag,hasMore",
        ),
      ).body as? JsonObject
      if (response == null) {
        problems += "$context page ${page + 1}: expected an object response"
        return all.values.toList()
      }
      response.objectElements("data", "$context page ${page + 1}", problems).objects
        .forEachIndexed { index, wrapper ->
          val message = wrapper["chatMessage"] as? JsonObject
          if (message == null || message.isEmpty()) {
            problems += "$context page ${page + 1} data[$index]: missing chatMessage object"
          }
          else all.putIfAbsent(message.text("id"), message)
        }
      if (response.flag("hasMore") == null) {
        problems += "$context page ${page + 1}: omitted pagination completeness flag"
        return all.values.toList()
      }
      if (response.flag("hasMore") == false) return all.values.toList()
      etag = response.text("etag")
      if (etag.isEmpty() || !cursors.add(etag)) {
        problems += "$context page ${page + 1}: pagination did not advance"
        return all.values.toList()
      }
    }
    problems += "$context exceeds $MAX_PAGES pages; evidence is incomplete"
    return all.values.toList()
  }

  private fun spaceDiscussion(channel: String, context: String, problems: MutableList<String>): List<ReviewMessage> {
    val all = mutableListOf<ReviewMessage>()
    val seen = mutableSetOf<String>()
    val cursors = mutableSetOf<String>()
    var start = ""
    for (page in 0 until MAX_PAGES) {
      val query = mutableMapOf(
        "channel" to "id:$channel",
        "sorting" to "FromOldestToNewest",
        "batchSize" to "$DISCUSSION_PAGE_SIZE",
        "\$fields" to "messages(id,text,author(name,details(className,user(id))),created),nextStartFromDate,orgLimitReached",
      )
      if (start.isNotEmpty()) query["startFromDate"] = start
      val response = get("/chats/messages", query).body as? JsonObject
      if (response == null) {
        problems += "$context page ${page + 1}: expected an object response"
        return all
      }
      if (response.flag("orgLimitReached") != false) {
        problems += "$context page ${page + 1}: history unavailable or limited"
        return all
      }
      val messages = response.objectElements("messages", "$context page ${page + 1}", problems)
      var fresh = 0
      messages.objects.forEachIndexed { index, message ->
        if (message.text("id").isEmpty()) {
          problems += "$context page ${page + 1} messages[$index]: missing message ID"
          return@forEachIndexed
        }
        if (!seen.add(message.text("id"))) return@forEachIndexed
        fresh++
        val author = message.obj("author")
        if (message.text("text").isNotEmpty() && !isBot(author.text("name"), author.obj("details").text("className"))) {
          val created = message.obj("created")
          val timestamp = created.text("iso").ifEmpty { Instant.ofEpochMilli(created.number("timestamp")).toString() }
          all += ReviewMessage(author.text("name"), message.text("text"), timestamp)
        }
      }
      if (messages.elementCount < DISCUSSION_PAGE_SIZE) return all
      val timestamp = response.obj("nextStartFromDate").number("timestamp")
      if (fresh <= 0 || timestamp <= 0) {
        problems += "$context page ${page + 1}: pagination did not advance"
        return all
      }
      start = Instant.ofEpochMilli(timestamp).toString()
      if (!cursors.add(start)) {
        problems += "$context page ${page + 1}: repeated date cursor"
        return all
      }
    }
    problems += "$context exceeds $MAX_PAGES pages; evidence is incomplete"
    return all
  }

  private data class ObjectElements(val objects: List<JsonObject>, val elementCount: Int)

  private fun JsonObject.objectElements(
    key: String,
    context: String,
    problems: MutableList<String>,
  ): ObjectElements {
    val elements = get(key) as? JsonArray
    if (elements == null) {
      problems += "$context: expected '$key' array"
      return ObjectElements(emptyList(), 0)
    }
    val objects = elements.mapIndexedNotNull { index: Int, element: JsonElement ->
      (element as? JsonObject).also {
        if (it == null) problems += "$context $key[$index]: expected object"
      }
    }
    return ObjectElements(objects, elements.size)
  }

  private fun get(endpoint: String, query: Map<String, String> = emptyMap()): CiHttpResponse =
    http.send(id, "GET", endpoint, query).also { response ->
      if (response.status == 404) throw NoSuchFileException("space source")
      check(response.status == 200) { "space API returned HTTP ${response.status}" }
    }

  private fun repositoryEndpoint(repository: ReviewRepository): String =
    "/projects/key:${urlPart(repository.owner)}/repositories/${urlPart(repository.repository)}"

  private fun expectObject(response: CiHttpResponse, vararg expected: Int): JsonObject {
    require(response.status in expected.toSet()) { "Space API returned HTTP ${response.status}" }
    return response.body as? JsonObject ?: error("Space API did not return an object")
  }

  private companion object {
    const val PAGE_SIZE = 100
    const val DISCUSSION_PAGE_SIZE = 50
    const val MAX_PAGES = 1000
  }
}
