// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.git.GitRepository
import org.jetbrains.qodana.edict.model.*
import org.jetbrains.qodana.edict.signals.SignalValidation
import java.nio.file.Path
import org.jetbrains.qodana.edict.model.FileRevision as SignalFileRevision

/** Import supplied labels as feedback, without inventing a correcting commit or PR discussion. */
internal fun seedBenchmarkInbox(repository: GitRepository, output: Path, specification: JsonObject, revision: String) {
    val rule = specification.string("ruleId")
    val description = specification.string("description")
    for ((field, label) in listOf("positiveExamples" to SignalLabel.POSITIVE, "negativeExamples" to SignalLabel.NEGATIVE)) {
        specification[field]?.jsonArray.orEmpty().forEachIndexed { index, element ->
            val example = json.decodeFromJsonElement<FileRevision>(element)
            val sourceRevision = example.revision.ifBlank { revision }
            val source = repository.fileAt(sourceRevision, example.path)
            val lineCount = source.count { it == '\n' } + if (source.isNotEmpty() && !source.endsWith('\n')) 1 else 0
            val ranges = example.expectedProblemRanges?.takeIf { it.isNotEmpty() } ?: listOf(LineRange(1, lineCount))
            // Benchmark ranges may include trailing context beyond EOF. Preserve them exactly,
            // while requiring each range to start on actual historical source.
            require(ranges.all { it.start >= 1 && it.end >= it.start && it.start <= lineCount }) {
                "Invalid required example range in $rule/$field/$index at $sourceRevision:${example.path}"
            }
            val origin = "benchmark/$rule/specification.json#/$field/$index"
            val key = "benchmark:$revision:$origin"
            val signal = Signal(stableSignalId(key), key,
                SignalFileRevision(example.path, sourceRevision, ranges.map { SignalRange(it.start, it.end) }),
                SignalSource(type = "SubmittedFeedback", diffPositiveToNegative = "", message = description,
                    url = "git:$revision:$origin"), label,
                description.replace(Regex("\\s+"), " ").trim().take(1000),
                provenance = Provenance(workItemId = origin))
            val content = json.encodeToJsonElement(signal)
            val path = "inbox/${signal.id}.json"
            SignalValidation.validate(path, content.toString())
            writeJson(output.resolve("state/$path"), content)
        }
    }
}
