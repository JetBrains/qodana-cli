// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.support

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.addJsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import kotlinx.serialization.json.putJsonObject
import org.jetbrains.qodana.edict.ci.CiHttpResponse
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.urlPart
import java.util.Base64

/**
 * The Space HTTP API of [repository]: requests exactly as the client sends them, and answers shaped as Space documents
 * them, holding only the fields the client reads. Reads are recorded from the real repository where they can be; these
 * pieces script the writes, and the reads whose answer the real repository doesn't give.
 */
internal class SpaceExchanges(private val repository: ReviewRepository) {
  private val project = "/projects/key:${repository.owner}"
  private val root = "$project/repositories/${repository.repository}"

  fun branchRead(branch: String): ProviderRequest = ProviderRequest("GET", "$root/branches/${urlPart("refs/heads/$branch")}")

  fun branchCreation(branch: String, revision: String): ProviderRequest {
    val body = buildJsonObject {
      put("head", "refs/heads/$branch")
      put("target", revision)
    }
    return ProviderRequest("POST", "$root/head", body = body)
  }

  fun fileList(revision: String, path: String): ProviderRequest =
    ProviderRequest("GET", "$root/files", mapOf("commit" to revision, "path" to path))

  /** The read of the content stored as [blob]. */
  fun fileContent(blob: String): ProviderRequest =
    ProviderRequest("GET", "$root/content", mapOf("blobId" to blob, "skip" to "0", "limit" to "16777216"))

  /** Commits [files], by repository-relative path, on top of the [base] revision to [branch]. */
  fun commit(branch: String, base: String, message: String, files: Map<String, String>): ProviderRequest {
    val body = buildJsonObject {
      put("baseCommit", base)
      put("targetBranch", "refs/heads/$branch")
      put("commitMessage", message)
      putJsonArray("files") {
        files.forEach { (path, content) ->
          addJsonObject {
            put("path", path)
            putJsonObject("content") {
              put("className", "GitFileContent.Base64")
              put("value", Base64.getEncoder().encodeToString(content.encodeToByteArray()))
            }
          }
        }
      }
    }
    return ProviderRequest("POST", "/projects/${repository.owner}/repositories/${repository.repository}/commit", body = body)
  }

  /** The search for reviews in [state] from [branch] into [target], a branch name without `refs/heads/`. */
  fun reviewSearch(branch: String, target: String, state: String): ProviderRequest = ProviderRequest(
    "GET",
    "$project/code-reviews",
    mapOf(
      "repository" to repository.repository,
      "sourceBranch" to branch,
      "targetBranch" to target,
      "state" to state,
      "\$skip" to "0",
      "\$top" to "100",
      "\$fields" to "data(review(id,number,state,branchPair(repository,sourceBranch,targetBranch)))",
    ),
  )

  fun reviewCreation(title: String, description: String, branch: String, target: String): ProviderRequest {
    val body = buildJsonObject {
      put("title", title)
      put("description", description)
      put("repository", repository.repository)
      put("sourceBranch", branch)
      put("targetBranch", target)
    }
    return ProviderRequest("POST", "$project/code-reviews/merge-requests", body = body)
  }

  /** Adds the reviewer named by [identifier], a profile id or a `username:` selector, to the review [reviewId]. */
  fun reviewerAssignment(reviewId: String, identifier: String): ProviderRequest = ProviderRequest(
    "POST",
    "$project/code-reviews/${urlPart(reviewId)}/participants/${urlPart(identifier)}",
    body = buildJsonObject { put("role", "Reviewer") },
  )

  /** A branch whose head is the commit [revision]. */
  fun branch(branch: String, revision: String): CiHttpResponse =
    answer(200, """{"branchInfo":{"head":"refs/heads/$branch","ref":"$revision"}}""")

  /** The listing of a file stored as [blob]. */
  fun files(path: String, blob: String): CiHttpResponse = answer(200, """[{"path":"$path","blob":"$blob","type":"FILE"}]""")

  /** The listing of a path that doesn't exist at the revision. */
  fun noFiles(): CiHttpResponse = answer(200, "[]")

  fun content(text: String): CiHttpResponse {
    val bytes = text.encodeToByteArray()
    return answer(200, """{"partBase64":"${Base64.getEncoder().encodeToString(bytes)}","totalSize":${bytes.size}}""")
  }

  /** A commit operation's result, which Space answers with HTTP 200 whether it succeeded or not. */
  fun commitResult(success: Boolean, message: String): CiHttpResponse =
    CiHttpResponse(200, buildJsonObject { put("success", success); put("message", message) })

  /** A review a search finds: the review [id] and [number] in [state], from [branch] into [target], as Space lists it. */
  fun review(id: String, number: Int, branch: String, target: String, state: String = "Opened"): JsonObject = buildJsonObject {
    putJsonObject("review") {
      put("id", id)
      put("number", number)
      put("state", state)
      putJsonObject("branchPair") {
        put("repository", repository.repository)
        put("sourceBranch", branch)
        put("targetBranch", target)
      }
    }
  }

  /** The [reviews] a search finds, in the order Space lists them. */
  fun reviews(vararg reviews: JsonObject): CiHttpResponse =
    CiHttpResponse(200, buildJsonObject { put("data", JsonArray(reviews.toList())) })

  fun mergeRequest(id: String, number: Int): CiHttpResponse =
    answer(200, """{"id":"$id","number":$number}""")
}

/**
 * HTTP 200 without a body: Space's answer to a write that has no result, such as a branch creation, and to a read of a
 * branch or review that doesn't exist.
 */
internal fun spaceEmpty(): CiHttpResponse = CiHttpResponse(200, JsonNull)

/** A Space error with its [code], such as `permission-denied`, and its [description]. */
internal fun spaceError(status: Int, code: String, description: String): CiHttpResponse =
  CiHttpResponse(status, buildJsonObject { put("error", code); put("error_description", description) })
