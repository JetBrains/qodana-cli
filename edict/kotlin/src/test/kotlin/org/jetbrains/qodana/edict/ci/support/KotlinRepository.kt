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
import org.jetbrains.qodana.edict.ci.SourceFixChanges

/** JetBrains/kotlin, which the integration tests read through the real API and the other tests replay. */
internal val KOTLIN = ReviewRepository(CiProviderId.GITHUB, "github.com", "JetBrains", "kotlin")

/** The build server that reports on JetBrains/kotlin pull requests as the user `KotlinBuild`, configured in lowercase. */
internal val KOTLIN_BOTS = setOf("kotlinbuild")

/** A merged pull request with two review threads, one of them with seven replies. */
internal val PULL_REQUEST_6007 = kotlinPullRequest(6007)

/** A pull request closed without merging. */
internal val PULL_REQUEST_5976 = kotlinPullRequest(5976)

/** A pull request open since 2017. */
internal val PULL_REQUEST_1393 = kotlinPullRequest(1393)

/** A pull request merged as one rebased commit of its four. */
internal val PULL_REQUEST_7832 = kotlinPullRequest(7832)

/** A pull request of three commits merged by a merge commit. */
internal val PULL_REQUEST_917 = kotlinPullRequest(917)

/** A pull request of one commit that changes 128 files and renames 7 of them. */
internal val PULL_REQUEST_7091 = kotlinPullRequest(7091)

/** A merged pull request that Copilot reviewed. */
internal val PULL_REQUEST_7185 = kotlinPullRequest(7185)

/** A number GitHub has no pull request for. */
internal const val MISSING_PULL_REQUEST = 999999

internal val MISSING_REVIEW = kotlinPullRequest(MISSING_PULL_REQUEST)

/** The same pull request number in a fork, which a client of [KOTLIN] must not read. */
internal val FORK_REVIEW = ReviewReference(CiProviderId.GITHUB, "fork", "kotlin", "6007", "https://github.com/fork/kotlin/pull/6007")

/** The code-owner table that the deleted account `ghost` posted on pull request 6007. */
internal const val CODE_OWNERS_6007 =
  "### Code Owners\n\n<table><tr><th>Rule</th><th>Owners</th><th>Approval</th></tr><tr>" +
    "<td><code>/\u200bcore/\u200bdescriptors/\u200b</code></td>" +
    "<td><details><summary><a href=\"https://github.com/orgs/JetBrains/teams/kotlin-frontend\">kotlin-frontend</a></summary>" +
    "<ul><li><a href=\"https://github.com/ligee\"><b><code>@ligee</code></b></a></li>" +
    "<li><a href=\"https://github.com/dzharkov\"><b><code>@dzharkov</code></b></a></li>" +
    "<li><a href=\"https://github.com/bnorm\"><b><code>@bnorm</code></b></a></li>" +
    "<li><a href=\"https://github.com/mglukhikh\"><b><code>@mglukhikh</code></b></a></li>" +
    "<li><a href=\"https://github.com/KvanTTT\"><b><code>@KvanTTT</code></b></a></li>" +
    "<li><a href=\"https://github.com/cypressious\"><b><code>@cypressious</code></b></a></li>" +
    "<li><a href=\"https://github.com/demiurg906\"><b><code>@demiurg906</code></b></a></li>" +
    "<li><a href=\"https://github.com/FenstonSingel\"><b><code>@FenstonSingel</code></b></a></li>" +
    "<li><a href=\"https://github.com/lunakoly\"><b><code>@lunakoly</code></b></a></li>" +
    "<li><a href=\"https://github.com/agamzikova\"><b><code>@agamzikova</code></b></a></li>" +
    "<li><a href=\"https://github.com/GrigoriiSolnyshkin\"><b><code>@GrigoriiSolnyshkin</code></b></a></li>" +
    "<li><a href=\"https://github.com/Aleksandra-Arsenteva\"><b><code>@Aleksandra-Arsenteva</code></b></a></li></ul></details></td>" +
    "<td align=\"center\">\u274c<br><b><code>UNASSIGNED</code></b></td></tr></table>\n<!-- CODE_OWNERS_REVIEW_COMMENT -->"

/**
 * The first messages of the discussion of pull request 6007 without bots, in GitHub's order: [CODE_OWNERS_6007] of
 * `ghost`, a person's `/safe-merge` command and the first review comment. Between them on GitHub are the code-owner
 * table of `kotlin-safemerge[bot]` and the quality-gate report of the build server `KotlinBuild`.
 */
internal val DISCUSSION_6007_START = listOf(CODE_OWNERS_6007, "/safe-merge", "Does it affect memory consumption?")

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

/** Two merged pull requests out of number order, a number GitHub doesn't have, and a pull request closed unmerged. */
internal val SEVERAL_NUMBERS_SELECTION = ReviewSelection(maxPrs = 4, prNumbers = listOf(9413, 9173, MISSING_PULL_REQUEST, 5976))

/** Pull requests closed on the weekend of 2026-10-31, when only pull request 9413 was merged. */
internal val WEEKEND_SELECTION = ReviewSelection(maxPrs = 10, startDate = "2026-10-31", endDate = "2026-11-01")

/**
 * The first two, in GitHub's order of their last update, of the pull requests merged on 2026-10-29 and 2026-10-30:
 * merged late on the last day, after 9413, merged seven minutes past the range.
 */
internal val FIRST_TWO_MERGED_SELECTION = ReviewSelection(maxPrs = 2, startDate = "2026-10-29", endDate = "2026-10-30")

/** A generated inspection, opened for review against [RELEASE_BRANCH]. */
internal val INSPECTION_CONTENT = recording("avoid-thread-sleep.inspection.kts")
internal const val INSPECTION_PATH = "inspections/avoid-thread-sleep.inspection.kts"
internal const val INSPECTION_TITLE = "Add Edict inspection avoid-thread-sleep"
internal const val INSPECTION_DESCRIPTION = "Reviewers asked five times to replace Thread.sleep with explicit synchronization."
internal const val REVIEWER = "reviewer"

/** The SHA-256 hex of [INSPECTION_CONTENT]. */
internal const val INSPECTION_DIGEST = "9cc450caa078baaa35ba265ebde3acedaeaba9ad74f328c9bb89133022b29d5a"

/** The inspection's branch: its cluster and the first 12 hex digits of [INSPECTION_DIGEST]. */
internal const val INSPECTION_BRANCH = "edict/avoid-thread-sleep-9cc450caa078"

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

/** A regular file and an executable one at [RELEASE_HEAD], fixed by [SOURCE_FIX_REQUEST]. */
internal const val FIXED_FILE = "libraries/stdlib/src/kotlin/util/Lazy.kt"
internal const val FIXED_SCRIPT = "gradlew"
internal val SOURCE_FIX = SourceFixChanges(
  RELEASE_HEAD,
  mapOf(FIXED_SCRIPT to "#!/usr/bin/env sh\n# Fixed by Edict\n", FIXED_FILE to "package kotlin\n// Fixed by Edict\n"),
)

/** The source fix's branch, named after its operation. */
internal const val SOURCE_FIX_BRANCH = "edict/weak-w-cccccccccccccccccccccccc"
internal const val SOURCE_FIX_TITLE = "Fix issues found by Edict inspection avoid-thread-sleep"
internal const val SOURCE_FIX_DESCRIPTION = "Replaces Thread.sleep with explicit synchronization where the inspection reports it."

internal val SOURCE_FIX_REQUEST = CreateReviewRequest(
  operationId = "w-cccccccccccccccccccccccc",
  clusterId = "avoid-thread-sleep",
  targetBranch = RELEASE_BRANCH,
  reviewer = REVIEWER,
  targetPath = "",
  inspectionDigest = INSPECTION_DIGEST,
  inspectionContent = "",
  description = SOURCE_FIX_DESCRIPTION,
  sourceFix = SOURCE_FIX,
)

/** The source fix analyzed at [RELEASE_PARENT], before the branch moved to [RELEASE_HEAD]. */
internal val STALE_SOURCE_FIX_REQUEST = SOURCE_FIX_REQUEST.copy(sourceFix = SOURCE_FIX.copy(baseRevision = RELEASE_PARENT))

/** `.bunch` at [RELEASE_HEAD], byte for byte, so a fix to it changes nothing. */
internal const val BUNCH_CONTENT = "193\n201\n192\n191_192\n183_191_192\nas34_183_191_192\nas35_191_192\nas36_192\nas40\nas41_201"
internal val UNCHANGED_SOURCE_FIX_REQUEST =
  SOURCE_FIX_REQUEST.copy(sourceFix = SourceFixChanges(RELEASE_HEAD, mapOf(".bunch" to BUNCH_CONTENT)))

/** A source fix that creates a file [RELEASE_HEAD] doesn't have. */
internal const val NEW_FILE = "libraries/stdlib/src/kotlin/util/Edict.kt"
internal val NEW_FILE_SOURCE_FIX_REQUEST =
  SOURCE_FIX_REQUEST.copy(sourceFix = SourceFixChanges(RELEASE_HEAD, mapOf(NEW_FILE to "package kotlin\n")))

/** The commit a scripted earlier run created on [SOURCE_FIX_BRANCH]. */
internal const val FIXED_COMMIT = "1111111111111111111111111111111111111111"

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
