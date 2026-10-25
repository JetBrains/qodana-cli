// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.github

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.add
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
import org.jetbrains.qodana.edict.ci.ReviewFetchResult
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.ReviewSelection
import org.jetbrains.qodana.edict.ci.ReviewState
import org.jetbrains.qodana.edict.ci.ReviewStatus
import org.jetbrains.qodana.edict.ci.ReviewThread
import org.jetbrains.qodana.edict.ci.MergedReview
import org.jetbrains.qodana.edict.ci.ReviewFileChange
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

  override fun fetch(selection: ReviewSelection): ReviewFetchResult {
    require(selection.provider == id)
    val root = repositoryEndpoint(selection.repositoryRef)
    val numbers = selection.prNumbers.toMutableList()
    val problems = mutableListOf<String>()
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
        val elements = response.body as? JsonArray
        if (elements == null) {
          problems += "GitHub PR discovery page $page: expected an array response"
          break
        }
        var old = false
        for ((index, element) in elements.withIndex()) {
          val item = element as? JsonObject
          if (item == null) {
            problems += "GitHub PR discovery page $page item[$index]: expected object"
            continue
          }
          try {
            val number = item.number("number").toInt()
            require(number > 0 && seen.add(number)) { "invalid or repeated PR number" }
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
          catch (e: Exception) {
            problems += "GitHub PR discovery page $page item[$index]: ${e.message ?: e.javaClass.simpleName}"
          }
        }
        if (old || numbers.size == selection.maxPrs || !response.hasNext && elements.size < PAGE_SIZE) break
        if (elements.isEmpty() || page == MAX_PAGES) {
          problems += "Incomplete GitHub PR discovery at page $page"
          break
        }
      }
    }
    val pullRequests = numbers.mapNotNull { number ->
      try {
        fetchPullRequest(root, number, selection)
      }
      catch (e: Exception) {
        problems += "GitHub PR $number: ${e.message ?: e.javaClass.simpleName}"
        null
      }
    }
    return ReviewFetchResult(pullRequests, problems)
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
        // TODO: file-level comments (`subject_type: file`) have no line, so the anchor becomes 0 and ReviewClient.fetch
        //  rejects the whole batch; skip or anchor them instead.
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
    val branch = request.branch
    if (request.sourceFix != null) ensureSourceFixBranch(request, branch)
    else ensureInspectionBranch(request, branch)

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
      put("title", request.title)
      put("head", branch)
      put("base", request.targetBranch)
      put("body", request.description)
    }, requireAuthentication = true), 201)
    val number = pullRequest["number"]?.jsonPrimitive?.content.orEmpty()
    require(number.toLongOrNull()?.let { it > 0 } == true) { "GitHub pull request has no number" }
    val reviewer = http.send(id, "POST", "$root/pulls/$number/requested_reviewers", body = buildJsonObject {
      put("reviewers", JsonArray(listOf(JsonPrimitive(request.reviewer))))
    }, requireAuthentication = true)
    require(reviewer.status in setOf(200, 201, 422)) { "GitHub reviewer assignment returned HTTP ${reviewer.status}" }
    return CreatedReview(number, pullRequest.text("html_url"))
  }

  private fun ensureInspectionBranch(request: CreateReviewRequest, branch: String) {
    val root = repositoryEndpoint(request.repository)
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
        put("message", request.title)
        put("content", Base64.getEncoder().encodeToString(request.inspectionContent))
        put("branch", branch)
        if (currentSha.isNotBlank()) put("sha", currentSha)
      }, requireAuthentication = true)
      require(response.status in setOf(200, 201)) { "GitHub inspection commit returned HTTP ${response.status}" }
    }
  }

  /** Commit all source files atomically, retaining the target tree's file modes. */
  private fun ensureSourceFixBranch(request: CreateReviewRequest, branch: String) {
    val changes = requireNotNull(request.sourceFix)
    val root = repositoryEndpoint(request.repository)
    val existing = http.send(id, "GET", "$root/git/ref/heads/${urlPart(branch)}", requireAuthentication = true)
    if (existing.status == 200) {
      val revision = existing.body.jsonObject.obj("object").text("sha")
      require(changes.files.all { (path, content) -> file(request.repository, revision, path) == content }) {
        "GitHub source-fix branch already exists with different content"
      }
      return
    }
    require(existing.status == 404) { "GitHub branch lookup returned HTTP ${existing.status}" }
    val base = expectObject(http.send(id, "GET", "$root/git/ref/heads/${urlPart(request.targetBranch)}", requireAuthentication = true), 200)
    require(base.obj("object").text("sha") == changes.baseRevision) {
      "Target branch changed since true-positive analysis; analyze the current revision before publishing fixes"
    }
    changes.files.forEach { (path, content) ->
      require(file(request.repository, changes.baseRevision, path) != content) { "Source fix for '$path' has no changes" }
    }
    val commit = expectObject(http.send(id, "GET", "$root/git/commits/${changes.baseRevision}", requireAuthentication = true), 200)
    val treeSha = commit.obj("tree").text("sha")
    val trees = mutableMapOf<String, JsonArray>()
    fun fileMode(path: String): String {
      var currentTree = treeSha
      val parts = path.split('/')
      for ((index, part) in parts.withIndex()) {
        val entries = trees.getOrPut(currentTree) {
          expectObject(http.send(id, "GET", "$root/git/trees/$currentTree", requireAuthentication = true), 200).getValue("tree").jsonArray
        }
        val entry = entries.singleOrNull { it.jsonObject.text("path") == part }?.jsonObject
          ?: error("Source fix path '$path' does not exist in the target tree")
        if (index == parts.lastIndex) {
          return entry.text("mode").also {
            require(entry.text("type") == "blob" && it in setOf("100644", "100755")) { "Source fixes require regular files" }
          }
        }
        require(entry.text("type") == "tree") { "Source fix path '$path' traverses a non-directory" }
        currentTree = entry.text("sha")
      }
      error("Invalid source fix path")
    }
    val modes = changes.files.keys.associateWith(::fileMode)
    val tree = expectObject(http.send(id, "POST", "$root/git/trees", body = buildJsonObject {
      put("base_tree", treeSha)
      put("tree", buildJsonArray {
        changes.files.toSortedMap().forEach { (path, content) -> add(buildJsonObject {
          put("path", path)
          put("type", "blob")
          put("mode", modes.getValue(path))
          put("content", content)
        }) }
      })
    }, requireAuthentication = true), 201)
    val created = expectObject(http.send(id, "POST", "$root/git/commits", body = buildJsonObject {
      put("message", request.title)
      put("tree", tree.text("sha"))
      put("parents", buildJsonArray { add(changes.baseRevision) })
    }, requireAuthentication = true), 201)
    expectObject(http.send(id, "POST", "$root/git/refs", body = buildJsonObject {
      put("ref", "refs/heads/$branch")
      put("sha", created.text("sha"))
    }, requireAuthentication = true), 201)
  }

  override fun review(reference: ReviewReference): ReviewStatus = readReview(reference, includeEvidence = false)

  override fun reviewWithEvidence(reference: ReviewReference): ReviewStatus = readReview(reference, includeEvidence = true)

  override fun mergedReview(reference: ReviewReference): MergedReview {
    require(reference.provider == id)
    val root = "/repos/${urlPart(reference.owner)}/${urlPart(reference.repository)}"
    val pull = expectObject(http.send(id, "GET", "$root/pulls/${urlPart(reference.id)}", requireAuthentication = true), 200)
    require(pull.text("merged_at").isNotBlank()) { "Review has not merged" }
    val merged = pull.text("merge_commit_sha")
    require(org.jetbrains.qodana.edict.signals.validRevision(merged)) { "Review has no exact merge revision" }
    val changes = mutableListOf<ReviewFileChange>()
    var before = ""
    for (page in 1..MAX_PAGES) {
      val response = get("$root/commits/$merged", mapOf("per_page" to "$PAGE_SIZE", "page" to "$page"))
      val commit = response.body.jsonObject
      require(commit.text("sha") == merged) { "Wrong merge revision returned" }
      val parents = commit.getValue("parents").jsonArray.map { it.jsonObject.text("sha") }
      require(parents.size in 1..2) { "Unsupported merge parent count" }
      // For multi-commit rebase merges merge_commit_sha names only the last commit. Do not treat it as the whole PR.
      require(parents.size == 2 || pull.number("commits").toInt() == 1) {
        "Cannot unambiguously identify the complete merged change for this single-parent, multi-commit PR"
      }
      if (page == 1) before = parents.first()
      require(before == parents.first()) { "Merge parent changed during pagination" }
      val files = commit.getValue("files").jsonArray.map { it.jsonObject }
      changes += files.map { ReviewFileChange(it.text("previous_filename").ifBlank { it.text("filename") }, it.text("filename")) }
      if (!response.hasNext && files.size < PAGE_SIZE) break
      require(page < MAX_PAGES && files.isNotEmpty()) { "Incomplete merged-change file list" }
    }
    require(org.jetbrains.qodana.edict.signals.validRevision(before) && changes.isNotEmpty() && changes.distinct().size == changes.size) {
      "Invalid merged-change evidence"
    }
    val messages = listOf("$root/issues/${urlPart(reference.id)}/comments", "$root/pulls/${urlPart(reference.id)}/reviews",
      "$root/pulls/${urlPart(reference.id)}/comments").flatMap(::pages)
      .filterNot { isBot(it.obj("user").text("login"), it.obj("user").text("type")) }
      .mapNotNull { it.text("body").takeIf(String::isNotBlank) }
    return MergedReview(pull.text("title"), messages, before, merged, changes)
  }

  private fun readReview(reference: ReviewReference, includeEvidence: Boolean): ReviewStatus {
    require(reference.provider == id)
    val root = "/repos/${urlPart(reference.owner)}/${urlPart(reference.repository)}"
    val pull = expectObject(http.send(id, "GET", "$root/pulls/${urlPart(reference.id)}", requireAuthentication = true), 200)
    val state = when {
      pull.text("merged_at").isNotBlank() -> ReviewState.MERGED
      pull.text("state") == "open" -> ReviewState.OPEN
      else -> ReviewState.CLOSED_UNMERGED
    }
    if (!includeEvidence && state != ReviewState.CLOSED_UNMERGED) return ReviewStatus(state)
    val endpoints = listOf("$root/issues/${urlPart(reference.id)}/comments", "$root/pulls/${urlPart(reference.id)}/reviews") +
      if (includeEvidence) listOf("$root/pulls/${urlPart(reference.id)}/comments") else emptyList()
    val messages = endpoints.flatMap(::pages)
      .filterNot { includeEvidence && isBot(it.obj("user").text("login"), it.obj("user").text("type")) }
      .mapNotNull { it.text("body").takeIf(String::isNotBlank) }
    return ReviewStatus(state, messages.joinToString("\n\n"))
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
