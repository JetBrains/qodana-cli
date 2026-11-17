// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.support

import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.CreateReviewRequest
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.SourceFixChanges

/** IJ/ultimate on Space, which the integration tests read through the real API and the other tests replay. */
internal val ULTIMATE = ReviewRepository(CiProviderId.SPACE, "jetbrains.team", "ij", "ultimate")
internal const val ULTIMATE_API = "https://jetbrains.team/api/http"

/** A review merged with five code discussions, in which the author answered every reviewer. */
internal val ULTIMATE_MERGED_REVIEW = ULTIMATE.reference("1Iu3Gp1HKnF6", "https://jetbrains.team/p/ij/reviews/224837")

/** A review closed without merging in 2025. */
internal val ULTIMATE_CLOSED_REVIEW = ULTIMATE.reference("4Flp0T3ETMCh", "https://jetbrains.team/p/ij/reviews/161809")

/** A review opened in May 2026 and left untouched since. */
internal val ULTIMATE_OPEN_REVIEW = ULTIMATE.reference("3mjARa4QCqdV", "https://jetbrains.team/p/ij/reviews/202718")

/** A merged review that Space lists but answers `null` for, as for a review that doesn't exist. */
internal val ULTIMATE_INVISIBLE_REVIEW = ULTIMATE.reference("3CNIEJ2jnTdL", "https://jetbrains.team/p/ij/reviews/232694")

/** A release branch of [ULTIMATE] that hasn't changed since 2020, so its head stays [RELEASE_191_HEAD]. */
internal const val RELEASE_191 = "191"
internal const val RELEASE_191_HEAD = "17795949cee5953ee3e18b810af8db74f641dc54"

/** The target revision of [ULTIMATE_MERGED_REVIEW], and a repository-relative file it changed. */
internal const val ULTIMATE_MERGED_REVIEW_BASE = "05086d8d08296dd055c7c7ec95fc76930fbd8b50"
internal const val ULTIMATE_STARLARK_FILE =
  "plugins/bazel/intellij.bazel.languages/src/org/jetbrains/bazel/languages/starlark/psi/StarlarkFile.kt"

/** The reviewer the requests ask for: a bare value, which Space reads as a profile id. */
internal const val ULTIMATE_REVIEWER = "reviewer"

/** The [INSPECTION_CONTENT] opened for review against [RELEASE_191]. */
internal val ULTIMATE_INSPECTION_REQUEST = CreateReviewRequest(
  operationId = "p-aaaaaaaaaaaaaaaaaaaaaaaa",
  clusterId = "avoid-thread-sleep",
  targetBranch = RELEASE_191,
  reviewer = ULTIMATE_REVIEWER,
  targetPath = INSPECTION_PATH,
  inspectionDigest = INSPECTION_DIGEST,
  inspectionContent = INSPECTION_CONTENT,
  description = INSPECTION_DESCRIPTION,
)

/** Two small files of [RELEASE_191], each replaced by the source fix, and the text of the first one at the release. */
internal const val ULTIMATE_BUILD_TXT = "community/build.txt"
internal const val ULTIMATE_GITIGNORE = "community/.gitignore"
internal const val ULTIMATE_BUILD_TXT_AT_191 = "191.SNAPSHOT\n"
internal val ULTIMATE_SOURCE_FIX =
  SourceFixChanges(RELEASE_191_HEAD, mapOf(ULTIMATE_BUILD_TXT to "191.7142\n", ULTIMATE_GITIGNORE to "/out/\n"))
internal const val ULTIMATE_SOURCE_FIX_TITLE = "Fix issues found by Edict inspection avoid-thread-sleep"
internal const val ULTIMATE_SOURCE_FIX_DESCRIPTION = "Fixes the five places the inspection reports."

/** The source fix's branch, named for its operation. */
internal const val ULTIMATE_SOURCE_FIX_BRANCH = "edict/weak-w-cccccccccccccccccccccccc"

internal val ULTIMATE_SOURCE_FIX_REQUEST = CreateReviewRequest(
  operationId = "w-cccccccccccccccccccccccc",
  clusterId = "avoid-thread-sleep",
  targetBranch = RELEASE_191,
  reviewer = ULTIMATE_REVIEWER,
  targetPath = "",
  inspectionDigest = INSPECTION_DIGEST,
  inspectionContent = "",
  description = ULTIMATE_SOURCE_FIX_DESCRIPTION,
  sourceFix = ULTIMATE_SOURCE_FIX,
)

/** The review Space opens for a request: its id and number are made up, as the tests never create a real one. */
internal const val ULTIMATE_NEW_REVIEW_ID = "2EdIcT0ReVu1"
internal const val ULTIMATE_NEW_REVIEW_NUMBER = 240001
internal val ULTIMATE_NEW_REVIEW =
  ULTIMATE.reference(ULTIMATE_NEW_REVIEW_ID, "https://jetbrains.team/p/ij/reviews/$ULTIMATE_NEW_REVIEW_NUMBER")

/** [ULTIMATE_SOURCE_FIX_REQUEST] whose [ULTIMATE_BUILD_TXT] is the file's text at its base. */
internal val ULTIMATE_UNCHANGED_SOURCE_FIX_REQUEST = ULTIMATE_SOURCE_FIX_REQUEST.copy(
  sourceFix = ULTIMATE_SOURCE_FIX.copy(files = ULTIMATE_SOURCE_FIX.files + (ULTIMATE_BUILD_TXT to ULTIMATE_BUILD_TXT_AT_191)),
)
