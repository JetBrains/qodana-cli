// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.support

import org.jetbrains.qodana.edict.edictnext.sha256Hex

/** A generated inspection, which the tests of both providers open for review. */
internal val INSPECTION_CONTENT = recording("avoid-thread-sleep.inspection.kts")
internal const val INSPECTION_PATH = "inspections/avoid-thread-sleep.inspection.kts"
internal const val INSPECTION_TITLE = "Add Edict inspection avoid-thread-sleep"
internal const val INSPECTION_DESCRIPTION = "Reviewers asked five times to replace Thread.sleep with explicit synchronization."
internal const val REVIEWER = "reviewer"
internal val INSPECTION_DIGEST = sha256Hex(INSPECTION_CONTENT)

/** The inspection's branch: its cluster and the first 12 hex digits of [INSPECTION_DIGEST]. */
internal const val INSPECTION_BRANCH = "edict/avoid-thread-sleep-9cc450caa078"
