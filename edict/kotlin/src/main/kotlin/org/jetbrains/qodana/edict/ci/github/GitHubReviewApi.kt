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
import org.jetbrains.qodana.edict.ci.MergedReview
import org.jetbrains.qodana.edict.ci.ReviewFileChange
import org.jetbrains.qodana.edict.ci.urlPart
import org.jetbrains.qodana.edict.signals.validRevision
import org.jetbrains.qodana.edict.signals.validSourcePath
import org.jetbrains.qodana.edict.common.number
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.sha256
import org.jetbrains.qodana.edict.common.text
import java.nio.file.NoSuchFileException
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneOffset
import java.util.Base64

/** The client of [repository], with `GITHUB_TOKEN` (or `GH_TOKEN`) from the server environment. */
internal fun gitHubReviewApi(repository: ReviewRepository, bots: Set<String>): GitHubReviewApi {
  // Pull requests link to github.com, so reviews under another host would never be owned.
  require(repository.host != "www.github.com") { "Name the repository as https://github.com/<owner>/<repository>, without www" }
  val token = System.getenv("GITHUB_TOKEN")?.takeIf(String::isNotBlank) ?: System.getenv("GH_TOKEN")
  require(!token.isNullOrBlank()) { "${repository.label()} requires GITHUB_TOKEN or GH_TOKEN in the server environment" }
  return GitHubReviewApi(repository, DefaultCiHttpTransport(CiProviderId.GITHUB, gitHubApiUrl(repository.host), token), bots)
}

/** The API of github.com, of GitHub Enterprise Cloud (`<sub>.ghe.com`) or, on any other host, of GitHub Enterprise Server. */
internal fun gitHubApiUrl(host: String): String = when {
  host == "github.com" -> "https://api.github.com"
  host.endsWith(".ghe.com") -> "https://api.$host"
  else -> "https://$host/api/v3"
}

internal class GitHubReviewApi(
  override val repository: ReviewRepository,
  private val http: CiHttpTransport,
  /** The logins, in any case, of accounts that post as users but are bots, such as a build server's. */
  private val bots: Set<String>,
) : ReviewApi {
  // TODO(R11): discovery pages and PR reads append to `numbers` and `problems` from nested loops; map each page and PR
  //  to its result (numbers or reviews, plus its own problems) and combine them at the end.
  override fun fetch(selection: ReviewSelection): ReviewFetchResult {
    val root = repositoryEndpoint()
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
        val lines = anchor.anchorLines()
        val messages = reviews.filter {
          it.number("id") == anchor.number("pull_request_review_id") && it.text("body").isNotBlank()
        }
          .filterNot { it.isBot() }
          .map { ReviewMessage(it.obj("user").text("login"), it.text("body"), it.text("submitted_at")) } +
          group.sortedWith(compareBy({ it.text("created_at") }, { it.number("id") }))
            .filterNot { it.isBot() }
            .map { ReviewMessage(it.obj("user").text("login"), it.text("body"), it.text("created_at")) }
        if (messages.isEmpty()) null else ReviewThread(
          "github-$number-$commentId",
          "${pullRequest.url}#discussion_r$commentId",
          anchor.text("path"),
          anchor.text("original_commit_id"),
          lines?.first,
          lines?.last,
          messages,
        )
      }.sortedWith(compareBy({ it.messages.first().createdAt }, { it.threadId }))
    return pullRequest.copy(threads = threads)
  }

  override fun file(revision: String, path: String): String {
    require(validRevision(revision) && validSourcePath(path)) { "Source reads require exact revision and repository-relative path" }
    return readFile(revision, path)
  }

  /** Reads without validation, for branch revisions this client resolved itself. */
  private fun readFile(revision: String, path: String): String {
    val response = http.send(
      "GET",
      "${repositoryEndpoint()}/contents/${path.split('/').joinToString("/", transform = ::urlPart)}",
      mapOf("ref" to revision),
    )
    // GitHub answers "Not Found" for a missing path, and names the ref when the revision doesn't exist.
    if (response.status == 404 && response.message() == "Not Found") throw NoSuchFileException(path)
    val value = expectObject(response, "GitHub file read", 200)
    require(value.text("type") == "file" && value.text("encoding") == "base64") {
      "GitHub did not return complete base64 file content"
    }
    val bytes = Base64.getDecoder().decode(value.text("content").replace("\n", "").replace("\r", ""))
    require(bytes.size.toLong() == value.number("size") && bytes.none { it == 0.toByte() }) {
      "Provider file content is invalid, binary or truncated"
    }
    return bytes.toString(Charsets.UTF_8)
  }

  override fun resolveTargetBranch(configured: String?): String {
    if (configured != null) return configured
    return expectObject(http.send("GET", repositoryEndpoint()), "GitHub repository read", 200)
      .text("default_branch")
      .also { require(it.isNotBlank()) { "GitHub repository has no default branch" } }
  }

  override fun findOrCreateReview(request: CreateReviewRequest): ReviewReference {
    val root = repositoryEndpoint()
    val branch = request.branch
    if (request.sourceFix != null) ensureSourceFixBranch(request, branch)
    else ensureInspectionBranch(request, branch)

    // TODO: `state=all` can return a closed or merged PR (an interrupted inspection promotion, or weak-signal fixes
    //  re-published with the same content), which is then reused and gets a reviewer request; GitHub may answer 422.
    //  Decide whether to reuse only open PRs, or at least skip the reviewer request on one that isn't open.
    val found = http.send(
      "GET",
      "$root/pulls",
      mapOf("state" to "all", "head" to "${repository.owner}:$branch", "base" to request.targetBranch),
    )
    expectStatus(found, "GitHub pull-request lookup", 200)
    val existing = found.body.jsonArray.firstOrNull()?.jsonObject
    val pullRequest = existing ?: expectObject(http.send("POST", "$root/pulls", body = buildJsonObject {
      put("title", request.title)
      put("head", branch)
      put("base", request.targetBranch)
      put("body", request.description)
    }), "GitHub pull-request creation", 201)
    val number = pullRequest["number"]?.jsonPrimitive?.content.orEmpty()
    require(number.toLongOrNull()?.let { it > 0 } == true) { "GitHub pull request has no number" }
    val reviewer = http.send("POST", "$root/pulls/$number/requested_reviewers", body = buildJsonObject {
      put("reviewers", JsonArray(listOf(JsonPrimitive(request.reviewer))))
    })
    expectStatus(reviewer, "GitHub reviewer assignment", 200, 201)
    return repository.reference(number, pullRequest.text("html_url"))
  }

  private fun ensureInspectionBranch(request: CreateReviewRequest, branch: String) {
    val root = repositoryEndpoint()
    val branchRef = http.send("GET", "$root/git/ref/heads/${urlPart(branch)}")
    if (branchRef.status == 404) {
      val base = expectObject(
        http.send("GET", "$root/git/ref/heads/${urlPart(request.targetBranch)}"), "GitHub target branch lookup", 200,
      )
      val sha = base["object"]?.jsonObject?.text("sha").orEmpty()
      require(sha.isNotBlank()) { "GitHub target branch has no head revision" }
      val created = http.send("POST", "$root/git/refs", body = buildJsonObject {
        put("ref", "refs/heads/$branch")
        put("sha", sha)
      })
      expectStatus(created, "GitHub branch creation", 201)
    }
    else expectStatus(branchRef, "GitHub branch lookup", 200)

    val contentEndpoint = "$root/contents/${request.targetPath.split('/').joinToString("/", transform = ::urlPart)}"
    val current = http.send("GET", contentEndpoint, mapOf("ref" to branch))
    var currentSha = ""
    var currentDigest = ""
    if (current.status == 200) {
      val value = current.body.jsonObject
      currentSha = value.text("sha")
      currentDigest = sha256(Base64.getDecoder().decode(value.text("content").replace("\n", "").replace("\r", "")))
    }
    else expectStatus(current, "GitHub target file lookup", 404)
    if (currentDigest != request.inspectionDigest) {
      val response = http.send("PUT", contentEndpoint, body = buildJsonObject {
        put("message", request.title)
        put("content", Base64.getEncoder().encodeToString(request.inspectionContent.encodeToByteArray()))
        put("branch", branch)
        if (currentSha.isNotBlank()) put("sha", currentSha)
      })
      expectStatus(response, "GitHub inspection commit", 200, 201)
    }
  }

  /** Commit all source files atomically, retaining the target tree's file modes. */
  private fun ensureSourceFixBranch(request: CreateReviewRequest, branch: String) {
    val changes = requireNotNull(request.sourceFix)
    val root = repositoryEndpoint()
    val existing = http.send("GET", "$root/git/ref/heads/${urlPart(branch)}")
    if (existing.status == 200) {
      val revision = existing.body.jsonObject.obj("object").text("sha")
      require(changes.files.all { (path, content) -> readFile(revision, path) == content }) {
        "GitHub source-fix branch already exists with different content"
      }
      return
    }
    expectStatus(existing, "GitHub branch lookup", 404)
    expectObject(http.send("GET", "$root/git/ref/heads/${urlPart(request.targetBranch)}"), "GitHub target branch lookup", 200)
    changes.files.forEach { (path, content) ->
      require(readFile(changes.baseRevision, path) != content) { "Source fix for '$path' has no changes" }
    }
    val commit = expectObject(http.send("GET", "$root/git/commits/${changes.baseRevision}"), "GitHub commit read", 200)
    val treeSha = commit.obj("tree").text("sha")
    val trees = mutableMapOf<String, JsonArray>()
    val modes = changes.files.keys.associateWith { fileMode(treeSha, it, trees) }
    val tree = expectObject(http.send("POST", "$root/git/trees", body = buildJsonObject {
      put("base_tree", treeSha)
      put("tree", buildJsonArray {
        changes.files.toSortedMap().forEach { (path, content) -> add(buildJsonObject {
          put("path", path)
          put("type", "blob")
          put("mode", modes.getValue(path))
          put("content", content)
        }) }
      })
    }), "GitHub tree creation", 201)
    val created = expectObject(http.send("POST", "$root/git/commits", body = buildJsonObject {
      put("message", request.title)
      put("tree", tree.text("sha"))
      put("parents", buildJsonArray { add(changes.baseRevision) })
    }), "GitHub commit creation", 201)
    expectObject(http.send("POST", "$root/git/refs", body = buildJsonObject {
      put("ref", "refs/heads/$branch")
      put("sha", created.text("sha"))
    }), "GitHub branch creation", 201)
  }

  /** The mode of the regular file at [path] in tree [treeSha], reading each tree once through [trees]. */
  private fun fileMode(treeSha: String, path: String, trees: MutableMap<String, JsonArray>): String {
    var currentTree = treeSha
    val parts = path.split('/')
    for ((index, part) in parts.withIndex()) {
      val entries = trees.getOrPut(currentTree) {
        expectObject(http.send("GET", "${repositoryEndpoint()}/git/trees/$currentTree"), "GitHub tree read", 200)
          .getValue("tree").jsonArray
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

  override fun review(reference: ReviewReference): ReviewStatus = readReview(reference, includeEvidence = false)

  override fun reviewWithEvidence(reference: ReviewReference): ReviewStatus = readReview(reference, includeEvidence = true)

  // TODO(R11): file pages are appended to `changes` in a loop; express the paging as a sequence of pages mapped to changes.
  override fun mergedReview(reference: ReviewReference): MergedReview {
    repository.requireOwns(reference)
    val root = repositoryEndpoint()
    val pull = expectObject(http.send("GET", "$root/pulls/${urlPart(reference.id)}"), "GitHub pull-request read", 200)
    require(pull.text("merged_at").isNotBlank()) { "Review has not merged" }
    val merged = pull.text("merge_commit_sha")
    require(validRevision(merged)) { "Review has no exact merge revision" }
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
    require(validRevision(before) && changes.isNotEmpty() && changes.distinct().size == changes.size) {
      "Invalid merged-change evidence"
    }
    val messages = listOf("$root/issues/${urlPart(reference.id)}/comments", "$root/pulls/${urlPart(reference.id)}/reviews",
      "$root/pulls/${urlPart(reference.id)}/comments").flatMap(::pages)
      .filterNot { it.isBot() }
      .mapNotNull { it.text("body").takeIf(String::isNotBlank) }
    return MergedReview(pull.text("title"), messages, before, merged, changes)
  }

  private fun readReview(reference: ReviewReference, includeEvidence: Boolean): ReviewStatus {
    repository.requireOwns(reference)
    val root = repositoryEndpoint()
    val pull = expectObject(http.send("GET", "$root/pulls/${urlPart(reference.id)}"), "GitHub pull-request read", 200)
    val state = when {
      pull.text("merged_at").isNotBlank() -> ReviewState.MERGED
      pull.text("state") == "open" -> ReviewState.OPEN
      else -> ReviewState.CLOSED_UNMERGED
    }
    if (!includeEvidence) return ReviewStatus(state)
    val endpoints = listOf(
      "$root/issues/${urlPart(reference.id)}/comments", "$root/pulls/${urlPart(reference.id)}/reviews", "$root/pulls/${urlPart(reference.id)}/comments",
    )
    val messages = endpoints.flatMap(::pages)
      .filterNot { it.isBot() }
      .mapNotNull { it.text("body").takeIf(String::isNotBlank) }
    return ReviewStatus(state, messages.joinToString("\n\n"))
  }

  /** Whether a bot wrote this comment or review: an app, which GitHub marks as `Bot`, or one of [bots]. */
  private fun JsonObject.isBot(): Boolean = obj("user").let { user ->
    user.text("type") == "Bot" || bots.any { it.equals(user.text("login"), ignoreCase = true) }
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
    http.send("GET", endpoint, query).also { expectStatus(it, "GitHub read of $endpoint", 200) }

  private fun repositoryEndpoint(): String =
    "/repos/${urlPart(repository.owner)}/${urlPart(repository.repository)}"

  private fun expectObject(response: CiHttpResponse, operation: String, vararg expected: Int): JsonObject {
    expectStatus(response, operation, *expected)
    return response.body as? JsonObject ?: error("$operation did not return an object")
  }

  private fun expectStatus(response: CiHttpResponse, operation: String, vararg expected: Int) {
    require(response.status in expected.toSet()) {
      val detail = response.message().take(1000).takeIf(String::isNotBlank)?.let { ": $it" }.orEmpty()
      "$operation returned HTTP ${response.status}$detail"
    }
  }

  private companion object {
    const val PAGE_SIZE = 100
    const val MAX_PAGES = 1000
  }
}

/**
 * The lines the thread of this review comment is anchored to, where GitHub has lines; null for a comment on the whole
 * file, which GitHub places on line 1.
 */
private fun JsonObject.anchorLines(): IntRange? {
  if (text("subject_type") == "file") return null
  val line = number("original_line").takeIf { it > 0 } ?: number("line")
  val start = number("original_start_line").takeIf { it > 0 } ?: number("start_line").takeIf { it > 0 } ?: line
  return start.toInt()..line.toInt()
}

/** The error message GitHub answered with; empty when there is none. */
private fun CiHttpResponse.message(): String = (body as? JsonObject)?.text("message").orEmpty()
