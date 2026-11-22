// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.support

import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.CiHttpResponse
import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.CreateReviewRequest
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.ReviewSelection

/** JetBrains/kotlin, which the integration tests read through the real API and the other tests replay. */
internal val KOTLIN = ReviewRepository(CiProviderId.GITHUB, "github.com", "JetBrains", "kotlin")

/** The build server that reports on JetBrains/kotlin pull requests as the user `KotlinBuild`, configured in lowercase. */
internal val KOTLIN_BOTS = setOf("kotlinbuild")

/** A merged pull request with two review threads: a comment with six replies, and one with a single reply. */
internal val PULL_REQUEST_6007 = kotlinPullRequest(6007)

/** A pull request closed without merging. */
internal val PULL_REQUEST_5976 = kotlinPullRequest(5976)

/** A pull request open since 2017. */
internal val PULL_REQUEST_1393 = kotlinPullRequest(1393)

/** A merged pull request that Copilot reviewed. */
internal val PULL_REQUEST_7185 = kotlinPullRequest(7185)

/** A number GitHub has no pull request for. */
internal const val MISSING_PULL_REQUEST = 999999

internal val MISSING_REVIEW = kotlinPullRequest(MISSING_PULL_REQUEST)

/** The same pull request number in a fork, which a client of [KOTLIN] must not read. */
internal val FORK_REVIEW = ReviewReference(CiProviderId.GITHUB, "fork", "kotlin", "6007", "https://github.com/fork/kotlin/pull/6007")

/** The commit that merged pull request 6007, and its parent. */
internal const val MERGE_6007 = "a8fd1e07347489fee69d9d5dfa6e49afa0856886"
internal const val BEFORE_6007 = "e4e2b3ccd42c1cc153ab7c4d24eb79f545796cff"

/** A full revision SHA GitHub has no commit for. */
internal const val MISSING_REVISION = "0000000000000000000000000000000000000000"

/** The file pull request 6007 changed. */
internal const val PATH_6007 = "core/descriptors/src/org/jetbrains/kotlin/descriptors/PackageFragmentProviderImpl.kt"

/** The merge commit of pull request 917, and its first parent. */
internal const val MERGE_917 = "fdb8e1026aee542994c512a524bf45672738cf68"
internal const val BEFORE_917 = "6cf90cfc4eb5474cdc655caf90da7e4889178148"

/** A file pull request 917 added. */
internal const val ADDED_BY_917 = "idea/testData/quickfix/addInlineToReifiedFunctionFix/basic.kt"

/** Paths at [RELEASE_HEAD]: a directory, a generated test of 1.2 MB, which GitHub serves without content, and an icon. */
internal const val DIRECTORY_PATH = "core/descriptors"
internal const val LARGE_FILE_PATH =
  "compiler/fir/resolve/tests/org/jetbrains/kotlin/fir/FirOldFrontendDiagnosticsTestGenerated.java"
internal const val BINARY_FILE_PATH = "idea/resources/org/jetbrains/kotlin/idea/icons/kotlin13.png"

/** A release branch of [KOTLIN] that hasn't changed since 2020, so its head stays [RELEASE_HEAD]. */
internal const val RELEASE_BRANCH = "1.3.70"
internal const val RELEASE_HEAD = "00708e800293156c9c4acaaaa7c06cb78e96ae69"

/** The parent of [RELEASE_HEAD]. */
internal const val RELEASE_PARENT = "5efcbacff39169cc98e2f778117c12028d4e4563"

/** The root tree of [RELEASE_HEAD]. */
internal const val RELEASE_TREE = "c66b387caf282113acb0c01a43e1048869961df9"

/** A branch [KOTLIN] doesn't have. */
internal const val MISSING_BRANCH = "edict-missing-branch"

/** Three merged pull requests in neither number order, a number GitHub doesn't have, and one closed unmerged. */
internal val SEVERAL_NUMBERS_SELECTION =
  ReviewSelection(maxPrs = 5, prNumbers = listOf(9413, 6007, 9173, MISSING_PULL_REQUEST, 5976))

/** Pull requests closed on the weekend of 2026-10-31, when only pull request 9413 was merged. */
internal val WEEKEND_SELECTION = ReviewSelection(maxPrs = 10, startDate = "2026-10-31", endDate = "2026-11-01")

/**
 * The first two, in GitHub's order of their last update, of the pull requests merged on 2026-10-29 and 2026-10-30:
 * merged late on the last day, after 9413, merged seven minutes past the range.
 */
internal val FIRST_TWO_MERGED_SELECTION = ReviewSelection(maxPrs = 2, startDate = "2026-10-29", endDate = "2026-10-30")

/**
 * The first ten pull requests merged on 2026-10-28: the first discovery page lists only pull requests updated later,
 * and the second holds 31 merged that day.
 */
internal val TWO_DISCOVERY_PAGES_SELECTION = ReviewSelection(maxPrs = 10, startDate = "2026-10-28", endDate = "2026-10-28")

/** The [INSPECTION_CONTENT] opened for review against [RELEASE_BRANCH]. */
internal val INSPECTION_REQUEST = CreateReviewRequest(
  operationId = "p-aaaaaaaaaaaaaaaaaaaaaaaa",
  clusterId = "avoid-thread-sleep",
  targetBranch = RELEASE_BRANCH,
  reviewer = REVIEWER,
  targetPath = INSPECTION_PATH,
  inspectionDigest = INSPECTION_DIGEST,
  inspectionContent = INSPECTION_CONTENT,
  description = INSPECTION_DESCRIPTION,
)

/** The inspection opened against a branch that doesn't exist. */
internal val MISSING_TARGET_REQUEST = INSPECTION_REQUEST.copy(targetBranch = MISSING_BRANCH)

/** The pull request a scripted creation answers with; GitHub won't reach this number for years. */
internal const val CREATED_PULL_REQUEST = 99999
internal val CREATED_REVIEW = kotlinPullRequest(CREATED_PULL_REQUEST)

/** Pull requests that scripted searches find from the inspection's branch, in GitHub's order. */
internal const val EXISTING_PULL_REQUEST = 99998
internal const val OLDER_PULL_REQUEST = 99997
internal val EXISTING_REVIEW = kotlinPullRequest(EXISTING_PULL_REQUEST)

private fun kotlinPullRequest(number: Int): ReviewReference =
  KOTLIN.reference("$number", "https://github.com/JetBrains/kotlin/pull/$number")

/** A GitHub failure with its [message]. */
internal fun gitHubFailure(status: Int, message: String): CiHttpResponse = CiHttpResponse(status, buildJsonObject {
  put("message", message)
  put("documentation_url", "https://docs.github.com/rest")
  put("status", "$status")
})

/** GitHub's message refusing a reviewer request for a user who isn't a collaborator. */
internal const val NOT_COLLABORATOR = "Reviews may only be requested from collaborators. One or more of the users or teams you " +
  "specified is not a collaborator of the JetBrains/kotlin repository."

/** GitHub's answer to creating a branch that exists. */
internal val EXISTING_BRANCH = gitHubFailure(422, "Reference already exists")

/** GitHub's answer to a pull-request creation when a pull request from [INSPECTION_BRANCH] appeared meanwhile. */
internal val PULL_REQUEST_EXISTS = answer(422, """{"message":"Validation Failed","errors":[{"resource":"PullRequest",""" +
  """"code":"custom","message":"A pull request already exists for JetBrains:$INSPECTION_BRANCH."}],""" +
  """"documentation_url":"https://docs.github.com/rest/pulls/pulls#create-a-pull-request","status":"422"}""")
