// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.space

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.CiHttpResponse
import org.jetbrains.qodana.edict.ci.CiHttpTransport
import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.DefaultCiHttpTransport
import org.jetbrains.qodana.edict.ci.CreateReviewRequest
import org.jetbrains.qodana.edict.ci.PullRequest
import org.jetbrains.qodana.edict.ci.ReviewApi
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
import org.jetbrains.qodana.edict.signals.validRevision
import org.jetbrains.qodana.edict.signals.validSourcePath
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.number
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import java.nio.file.NoSuchFileException
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneOffset
import java.util.Base64

/** The client of [repository], with `SPACE_TOKEN` from the server environment. */
internal fun spaceReviewApi(repository: ReviewRepository): SpaceReviewApi {
  val token = System.getenv("SPACE_TOKEN")
  require(!token.isNullOrBlank()) { "${repository.label()} requires SPACE_TOKEN in the server environment" }
  return SpaceReviewApi(repository, DefaultCiHttpTransport(CiProviderId.SPACE, "https://${repository.host}/api/http", token))
}


internal class SpaceReviewApi(
  override val repository: ReviewRepository,
  private val http: CiHttpTransport,
) : ReviewApi {
  // TODO(R11): discovery pages and PR reads append to `numbers` and `problems` from nested loops; map each page and PR
  //  to its result (numbers or reviews, plus its own problems) and combine them at the end.
  override fun fetch(selection: ReviewSelection): ReviewFetchResult {
    val root = "/projects/key:${urlPart(repository.owner)}/code-reviews"
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
            "repository" to repository.repository,
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
    require(pair.text("repository") == repository.repository && pair.flag("isMerged") != null) {
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
          "https://${repository.host}/im/review/${urlPart(value.text("id"))}?message=${urlPart(message.text("id"))}&channel=${urlPart(feed)}",
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
      "https://${repository.host}/p/${urlPart(repository.owner)}/reviews/$number",
      value.text("title"),
      value.text("description"),
      pair.obj("targetBranchInfo").text("ref"),
      pair.text("sourceBranchRef"),
      value.number("timestamp"),
      threads,
    )
  }

  override fun file(revision: String, path: String): String {
    require(validRevision(revision) && validSourcePath(path)) { "Source reads require exact revision and repository-relative path" }
    return readFile(revision, path)
  }

  /** Reads without validation, for branch revisions this client resolved itself. */
  private fun readFile(revision: String, path: String): String {
    val root = repositoryEndpoint()
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

  override fun resolveTargetBranch(configured: String?): String {
    if (configured != null) return configured
    return expectObject(http.send("GET", repositoryEndpoint()), 200)
      .obj("defaultBranch").text("head")
      .also { require(it.isNotBlank()) { "Space repository has no default branch" } }
  }

  override fun findOrCreateReview(request: CreateReviewRequest): ReviewReference {
    val branch = request.branch
    val branchHead = "refs/heads/$branch"
    if (request.sourceFix != null) ensureSourceFixBranch(request, branchHead)
    else ensureInspectionBranch(request, branchHead)

    val reviewsRoot = "/projects/key:${urlPart(repository.owner)}/code-reviews"
    val review = findReview(reviewsRoot, request, branch) ?: expectObject(http.send(
      "POST",
      "$reviewsRoot/merge-requests",
      body = buildJsonObject {
        put("title", request.title)
        put("description", request.description)
        put("repository", repository.repository)
        put("sourceBranch", branch)
        put("targetBranch", request.targetBranch)
      },
    ), 200, 201)
    val reviewId = review.text("id")
    require(reviewId.isNotBlank()) { "Space code review has no id" }
    var reviewer = assignReviewer(reviewsRoot, reviewId, request.reviewer)
    // A bare value is interpreted as a profile ID. Also accept configured usernames.
    if (reviewer.status == 404 && ':' !in request.reviewer) {
      reviewer = assignReviewer(reviewsRoot, reviewId, "username:${request.reviewer}")
    }
    expectStatus(reviewer, "Space reviewer assignment", 200, 201)
    val number = review.number("number")
    require(number > 0) { "Space code review has no number" }
    return repository.reference(reviewId, "https://${repository.host}/p/${urlPart(repository.owner)}/reviews/$number")
  }

  private fun assignReviewer(reviewsRoot: String, reviewId: String, identifier: String): CiHttpResponse = http.send(
    "POST", "$reviewsRoot/${urlPart(reviewId)}/participants/${urlPart(identifier)}",
    body = buildJsonObject { put("role", "Reviewer") },
  )

  private fun ensureInspectionBranch(request: CreateReviewRequest, branchHead: String) {
    val endpoint = repositoryEndpoint()
    val existingBranch = readBranch(endpoint, branchHead)
    val existingRevision = existingBranch?.obj("branchInfo")?.text("ref")?.also {
      require(it.isNotBlank()) { "Space inspection branch has no revision" }
    }
    val existingContent = existingRevision?.let { revision ->
      try { readFile(revision, request.targetPath) }
      catch (_: NoSuchFileException) { null }
    }
    if (existingContent != request.inspectionContent) {
      val base = requireNotNull(readBranch(endpoint, request.targetBranch)) { "Space target branch does not exist" }
      val revision = base.obj("branchInfo").text("ref")
      require(revision.isNotBlank()) { "Space target branch has no revision" }
      require(existingBranch == null || existingRevision == revision) {
        "Space inspection branch already exists with different content"
      }
      if (existingBranch == null) {
        val created = http.send("POST", "$endpoint/head", body = buildJsonObject {
          put("head", branchHead)
          put("target", revision)
        })
        expectStatus(created, "Space inspection branch creation", 200, 201)
      }
      // Unlike the other repository APIs, commit expects the plain project key, not a project selector.
      val commitEndpoint = "/projects/${urlPart(repository.owner)}/repositories/${urlPart(repository.repository)}/commit"
      val commit = expectObject(http.send("POST", commitEndpoint, body = buildJsonObject {
        put("baseCommit", revision)
        put("targetBranch", branchHead)
        put("commitMessage", request.title)
        put("files", buildJsonArray {
          add(buildJsonObject {
            put("path", request.targetPath)
            put("content", buildJsonObject {
              put("className", "GitFileContent.Base64")
              put("value", Base64.getEncoder().encodeToString(request.inspectionContent.encodeToByteArray()))
            })
          })
        })
      }), 200, 201)
      require(commit.flag("success") == true) { "Space inspection commit failed: ${commit.text("message").take(1000)}" }
    }
  }

  private fun ensureSourceFixBranch(request: CreateReviewRequest, branchHead: String) {
    val changes = requireNotNull(request.sourceFix)
    val endpoint = repositoryEndpoint()
    val existing = readBranch(endpoint, branchHead)
    val revision = existing?.obj("branchInfo")?.text("ref")
    if (revision != null && revision != changes.baseRevision) {
      require(changes.files.all { (path, content) -> readFile(revision, path) == content }) {
        "Space source-fix branch already exists with different content"
      }
      return
    }
    requireNotNull(readBranch(endpoint, request.targetBranch)) { "Space target branch does not exist" }
    changes.files.forEach { (path, content) ->
      require(readFile(changes.baseRevision, path) != content) { "Source fix for '$path' has no changes" }
    }
    if (existing == null) {
      expectStatus(http.send("POST", "$endpoint/head", body = buildJsonObject {
        put("head", branchHead)
        put("target", changes.baseRevision)
      }), "Space source-fix branch creation", 200, 201)
    }
    val commitEndpoint = "/projects/${urlPart(repository.owner)}/repositories/${urlPart(repository.repository)}/commit"
    val result = expectObject(http.send("POST", commitEndpoint, body = buildJsonObject {
      put("baseCommit", changes.baseRevision)
      put("targetBranch", branchHead)
      put("commitMessage", request.title)
      put("files", buildJsonArray {
        changes.files.toSortedMap().forEach { (path, content) -> add(buildJsonObject {
          put("path", path)
          put("content", buildJsonObject {
            put("className", "GitFileContent.Base64")
            put("value", Base64.getEncoder().encodeToString(content.encodeToByteArray()))
          })
        }) }
      })
    }), 200, 201)
    require(result.flag("success") == true) { "Space source-fix commit failed: ${result.text("message").take(1000)}" }
  }

  private fun readBranch(repository: String, branch: String): JsonObject? {
    val head = if (branch.startsWith("refs/heads/")) branch else "refs/heads/$branch"
    val response = http.send("GET", "$repository/branches/${urlPart(head)}")
    if (response.status == 404) return null
    expectStatus(response, "Space branch lookup", 200)
    return if (response.body == JsonNull) null else response.body.jsonObject
  }

  private fun findReview(root: String, request: CreateReviewRequest, branch: String): JsonObject? {
    for (state in listOf("Opened", "Closed")) {
      for (page in 0 until MAX_PAGES) {
        val response = expectObject(http.send("GET", root, mapOf(
          "repository" to repository.repository,
          "sourceBranch" to branch,
          "targetBranch" to request.targetBranch.removePrefix("refs/heads/"),
          "state" to state,
          "\$skip" to "${page * PAGE_SIZE}",
          "\$top" to "$PAGE_SIZE",
          "\$fields" to "data(review(id,number,branchPair(repository,sourceBranch,targetBranch)))",
        )), 200)
        val entries = response["data"] as? JsonArray ?: error("Space review lookup returned no data array")
        for (entry in entries) {
          val review = entry.jsonObject.obj("review")
          val pair = review.obj("branchPair")
          if (pair.text("repository") == repository.repository &&
            pair.text("sourceBranch").removePrefix("refs/heads/") == branch &&
            pair.text("targetBranch").removePrefix("refs/heads/") == request.targetBranch.removePrefix("refs/heads/")) {
            return review
          }
        }
        if (entries.size < PAGE_SIZE) break
        check(page < MAX_PAGES - 1) { "Space review lookup exceeds $MAX_PAGES pages" }
      }
    }
    return null
  }

  override fun review(reference: ReviewReference): ReviewStatus = readReview(reference, includeEvidence = false)

  override fun reviewWithEvidence(reference: ReviewReference): ReviewStatus = readReview(reference, includeEvidence = true)

  private fun readReview(reference: ReviewReference, includeEvidence: Boolean): ReviewStatus {
    repository.requireOwns(reference)
    val response = expectObject(
      http.send(
        "GET",
        "/projects/key:${urlPart(repository.owner)}/code-reviews/${urlPart(reference.id)}",
        query = mapOf("\$fields" to ("state,branchPair(isMerged)" + if (includeEvidence) ",feedChannelId" else "")),
      ),
      200,
    )
    val state = when {
      response.text("state").uppercase() in setOf("OPEN", "OPENED") -> ReviewState.OPEN
      response.text("state").uppercase() == "MERGED" || response.obj("branchPair").flag("isMerged") == true -> ReviewState.MERGED
      else -> ReviewState.CLOSED_UNMERGED
    }
    if (!includeEvidence) return ReviewStatus(state)
    val feed = response.text("feedChannelId")
    require(feed.isNotBlank()) { "Space review lacks discussion feed" }
    val problems = mutableListOf<String>()
    val messages = mutableListOf<String>()
    val discussions = mutableSetOf<String>()
    spaceFeed(feed, "Space review ${reference.id} feed", problems).forEach { message ->
      val author = message.obj("projectedItem").obj("author")
      if (!isBot(author.text("name"), author.obj("details").text("className"))) {
        message.text("text").takeIf(String::isNotBlank)?.let(messages::add)
      }
      val details = message.obj("details")
      if (details.text("className") == "CodeDiscussionAddedFeedEvent") {
        val channel = details.obj("codeDiscussion").obj("channel").text("id")
        require(channel.isNotBlank()) { "Space review discussion lacks channel" }
        if (discussions.add(channel)) {
          messages += spaceDiscussion(channel, "Space review ${reference.id} discussion $channel", problems).map { it.body }
        }
      }
    }
    require(problems.isEmpty()) { "Incomplete Space review feedback: ${problems.joinToString("; ")}" }
    return ReviewStatus(state, messages.joinToString("\n\n"))
  }

  // TODO(R11): `spaceFeed` and `spaceDiscussion` fill the caller's `problems` list as an out-parameter, which
  //  `fetchPullRequest` and `readReview` pass along; return the messages together with their problems instead.
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
    http.send("GET", endpoint, query).also { response ->
      if (response.status == 404) throw NoSuchFileException("space source")
      check(response.status == 200) { "space API returned HTTP ${response.status}" }
    }

  private fun repositoryEndpoint(): String =
    "/projects/key:${urlPart(repository.owner)}/repositories/${urlPart(repository.repository)}"

  private fun expectObject(response: CiHttpResponse, vararg expected: Int): JsonObject {
    expectStatus(response, "Space API", *expected)
    return response.body as? JsonObject ?: error("Space API did not return an object")
  }

  private fun expectStatus(response: CiHttpResponse, operation: String, vararg expected: Int) {
    require(response.status in expected.toSet()) {
      val body = response.body as? JsonObject
      val detail = body?.text("error_description").orEmpty().ifBlank { body?.text("message").orEmpty() }
      "$operation returned HTTP ${response.status}" + detail.take(1000).takeIf(String::isNotBlank)?.let { ": $it" }.orEmpty()
    }
  }

  private companion object {
    const val PAGE_SIZE = 100
    const val DISCUSSION_PAGE_SIZE = 50
    const val MAX_PAGES = 1000
  }
}
