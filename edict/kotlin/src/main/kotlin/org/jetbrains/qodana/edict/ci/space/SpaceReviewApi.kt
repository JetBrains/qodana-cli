// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.space

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.intOrNull
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
internal fun spaceReviewApi(repository: ReviewRepository, bots: Set<String>): SpaceReviewApi {
  val token = System.getenv("SPACE_TOKEN")
  require(!token.isNullOrBlank()) { "${repository.label()} requires SPACE_TOKEN in the server environment" }
  return SpaceReviewApi(repository, DefaultCiHttpTransport(CiProviderId.SPACE, "https://${repository.host}/api/http", token), bots)
}


internal class SpaceReviewApi(
  override val repository: ReviewRepository,
  private val http: CiHttpTransport,
  /** The usernames, in any case, of accounts that post as users but are bots. */
  private val bots: Set<String>,
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
      // Space keeps no merge time: `timestamp` is the review's last change, which for a merged review is its merge;
      // comments, resolved discussions and cherry-picks after the merge leave it.
      // `to` bounds the creation date, which every review merged by the end date meets, and `LastUpdatedDesc` sorts
      // by `timestamp`, so the first review changed before the start date ends the range.
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
            "\$fields" to "data(review(number,timestamp))",
          ),
        ).body as? JsonObject
        if (response == null) {
          problems += "Space review discovery page ${page + 1}: expected an object response"
          break
        }
        val data = response.objectElements("data", "Space review discovery page ${page + 1}", problems)
        var old = false
        for ((index, wrapper) in data.objects) {
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
    val value = reviewRecord(
      "number:$number",
      "id,number,state,timestamp,title,description,feedChannelId," +
        "branchPair(repository,isMerged,sourceBranchRef,targetBranchInfo(ref))",
    )
    require(value.number("number").toInt() == number) { "Space answered with another review" }
    val pair = value.obj("branchPair")
    require(pair.isNotEmpty()) { "Space review is a review of commits, not a merge request" }
    require(pair.text("repository") == repository.repository) { "Space review of another repository" }
    if (value.reviewState() != ReviewState.MERGED) return null
    if (!selection.containsDate(value.number("timestamp"))) return null
    val feed = value.text("feedChannelId")
    require(feed.isNotEmpty()) { "Space review lacks discussion feed" }
    val threads = spaceFeed(feed, "Space PR $number feed", problems).mapIndexedNotNull { index, message ->
      try {
        val details = message.obj("details")
        if (details.text("className") != "CodeDiscussionAddedFeedEvent") return@mapIndexedNotNull null
        val discussion = details.obj("codeDiscussion")
        val channel = discussion.obj("channel").text("id")
        require(channel.isNotEmpty()) { "discussion lacks channel" }
        val messages = spaceDiscussion(channel, "Space PR $number discussion $channel", problems)
        if (messages.isEmpty()) return@mapIndexedNotNull null
        val anchor = discussion.obj("anchor")
        val lines = anchor.anchorLines(discussion.obj("endAnchor"))
        ReviewThread(
          "space-$number-${message.text("id")}",
          "https://${repository.host}/im/review/${urlPart(value.text("id"))}?message=${urlPart(message.text("id"))}&channel=${urlPart(feed)}",
          anchor.text("filename").removePrefix("/"),
          anchor.text("revision"),
          lines?.first,
          lines?.last,
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
    // Space answers 404 for a revision it doesn't have, which the read reports, and no file for a missing path.
    val files = get("$root/files", mapOf("commit" to revision, "path" to path)).body as? JsonArray
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

  // Space answers the default branch as a ref, refs/heads/<name>.
  override fun resolveTargetBranch(configured: String?): String =
    configured ?: expectObject(http.send("GET", repositoryEndpoint()), 200).obj("defaultBranch").text("head")
      .removePrefix("refs/heads/").also { require(it.isNotBlank()) { "Space repository has no default branch" } }

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
    val number = review.number("number")
    require(number > 0) { "Space code review has no number" }
    var reviewer = assignReviewer(reviewsRoot, reviewId, request.reviewer)
    // A bare value is interpreted as a profile ID. Also accept configured usernames.
    if (reviewer.status == 404 && ':' !in request.reviewer) {
      reviewer = assignReviewer(reviewsRoot, reviewId, "username:${request.reviewer}")
    }
    expectStatus(reviewer, "Space reviewer assignment", 200, 201)
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
          "targetBranch" to request.targetBranch,
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
            pair.text("targetBranch").removePrefix("refs/heads/") == request.targetBranch) {
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
    val fields = "state,branchPair(isMerged)" + if (includeEvidence) ",feedChannelId" else ""
    val response = reviewRecord("id:${urlPart(reference.id)}", fields)
    val state = response.reviewState()
    if (!includeEvidence) return ReviewStatus(state)
    val feed = response.text("feedChannelId")
    require(feed.isNotBlank()) { "Space review lacks discussion feed" }
    val problems = mutableListOf<String>()
    // The feed also holds every message of the review's code discussions, under the same id, so it lists each once.
    val messages = spaceFeed(feed, "Space review ${reference.id} feed", problems).filter { it.isPersonText() }
    require(problems.isEmpty()) { "Incomplete Space review feedback: ${problems.joinToString("; ")}" }
    return ReviewStatus(state, messages.joinToString("\n\n") { it.text("text") })
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
          "\$fields" to "data(chatMessage(id,text,author(name,details(className))," +
            "details(className,codeDiscussion(channel(id),anchor(filename,line,oldLine,revision),endAnchor(line,oldLine))))),etag,hasMore",
        ),
      ).body as? JsonObject
      if (response == null) {
        problems += "$context page ${page + 1}: expected an object response"
        return all.values.toList()
      }
      response.objectElements("data", "$context page ${page + 1}", problems).objects
        .forEach { (index, wrapper) ->
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
        "\$fields" to "messages(id,text,details(className),author(name,details(className)),created)," +
          "nextStartFromDate,orgLimitReached",
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
      messages.objects.forEach { (index, message) ->
        if (message.text("id").isEmpty()) {
          problems += "$context page ${page + 1} messages[$index]: missing message ID"
          return@forEach
        }
        if (!seen.add(message.text("id"))) return@forEach
        fresh++
        if (message.isPersonText()) {
          val created = message.obj("created")
          val timestamp = created.text("iso").ifEmpty { Instant.ofEpochMilli(created.number("timestamp")).toString() }
          all += ReviewMessage(message.obj("author").text("name"), message.text("text"), timestamp)
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

  /** The [objects] of an array of [elementCount] elements, each with its index in the array. */
  private data class ObjectElements(val objects: List<IndexedValue<JsonObject>>, val elementCount: Int)

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
    val objects = elements.withIndex().mapNotNull { (index, element) ->
      (element as? JsonObject)?.let { IndexedValue(index, it) }.also {
        if (it == null) problems += "$context $key[$index]: expected object"
      }
    }
    return ObjectElements(objects, elements.size)
  }

  /**
   * A person's text message: Space marks events and state changes, such as a resolved discussion, with other content
   * classes, and gives applications and built-in services, such as "RubyMine Workflow" and "JetBrains Space", other
   * author classes than users. Users named in [bots] aren't people either.
   */
  private fun JsonObject.isPersonText(): Boolean =
    obj("details").text("className") == "M2TextItemContent" && text("text").isNotBlank() &&
      obj("author").let { author ->
        author.obj("details").text("className") == "CUserPrincipalDetails" &&
          bots.none { it.equals(author.text("name"), ignoreCase = true) }
      }

  private fun get(endpoint: String, query: Map<String, String> = emptyMap()): CiHttpResponse =
    http.send("GET", endpoint, query).also { expectStatus(it, "Space API", 200) }

  /** The [fields] of the review named by [selector], `id:` or `number:` and the path-encoded id or number. */
  private fun reviewRecord(selector: String, fields: String): JsonObject =
    get("/projects/key:${urlPart(repository.owner)}/code-reviews/$selector", mapOf("\$fields" to fields)).body as? JsonObject
      // Space answers null for a review that doesn't exist or that the token may not read.
      ?: throw IllegalArgumentException("Space review $selector is missing or not visible")

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

/**
 * The 1-based lines of a discussion from this anchor to [end], where Space has lines; null for a discussion of the
 * whole file. Space counts lines from 0, and anchors a line that the change removed by its `oldLine` in the anchor's
 * revision, which is then the one before the change.
 */
private fun JsonObject.anchorLines(end: JsonObject): IntRange? {
  val side = if (line("line") != null) "line" else "oldLine"
  val start = line(side) ?: return null
  return start + 1..(end.line(side) ?: start) + 1
}

/** The 0-based line under [key]; null where Space has none. */
private fun JsonObject.line(key: String): Int? = (get(key) as? JsonPrimitive)?.intOrNull

/**
 * The state of a review answered with `state` and `branchPair(isMerged)`. A review of commits has no branch pair, so
 * once closed it fails as having no merge status.
 */
private fun JsonObject.reviewState(): ReviewState = when (text("state")) {
  "Opened" -> ReviewState.OPEN
  "Closed" -> when (obj("branchPair").flag("isMerged")) {
    true -> ReviewState.MERGED
    false -> ReviewState.CLOSED_UNMERGED
    null -> throw IllegalArgumentException("Space review has no merge status")
  }
  else -> throw IllegalArgumentException("Space review has the unknown state '${text("state")}'")
}
