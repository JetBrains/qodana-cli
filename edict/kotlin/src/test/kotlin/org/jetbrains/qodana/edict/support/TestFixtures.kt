package org.jetbrains.qodana.edict.support

import org.jetbrains.qodana.edict.common.randomId
import org.jetbrains.qodana.edict.common.runProcess
import org.jetbrains.qodana.edict.git.CommitSignalExtractor
import org.jetbrains.qodana.edict.git.GitRepository
import org.jetbrains.qodana.edict.git.SignalFinding
import org.jetbrains.qodana.edict.model.*
import org.jetbrains.qodana.edict.skills.Registry
import org.jetbrains.qodana.edict.store.Store
import java.nio.file.Files
import java.nio.file.Path

internal const val fixturePath = "module/src/Equality.java"
internal const val beforeSource = "class Equality {\n  boolean same(String a, String b) {\n    return a == b;\n  }\n}\n"
internal const val afterSource =
    "class Equality {\n  boolean same(String a, String b) {\n    return java.util.Objects.equals(a, b);\n  }\n}\n"

internal fun gitFixture(directory: Path): GitRepository {
    Files.createDirectories(directory)
    fun git(vararg args: String) = runProcess(directory, listOf("git") + args)
    git("init", "-q")
    git("config", "user.name", "Edict Test")
    git("config", "user.email", "edict-test@example.invalid")
    git("config", "commit.gpgsign", "false")
    val source = directory.resolve(fixturePath)
    Files.createDirectories(source.parent)
    Files.writeString(source, beforeSource)
    git("add", "."); git("commit", "-qm", "Add string comparison")
    Files.writeString(source, afterSource)
    git("add", "."); git(
        "commit",
        "-qm",
        "Fix string equality: compare values rather than object identity, including nulls"
    )
    return GitRepository(directory)
}

/** Deliberately scripted semantic decision for offline protocol tests; LiveCommitExtractionTest exercises a real model. */
internal fun fixtureSignals(repository: GitRepository): List<Signal> =
    CommitSignalExtractor(repository) { commit, repo ->
        check(repo.fileAt(commit.parentRevision, fixturePath) == beforeSource)
        check(repo.fileAt(commit.commitRevision, fixturePath) == afterSource)
        listOf(
            SignalFinding(
                SignalLabel.POSITIVE,
                fixturePath,
                listOf(SignalRange(3, 3)),
                "String reference equality does not compare string values"
            ),
            SignalFinding(
                SignalLabel.NEGATIVE,
                fixturePath,
                listOf(SignalRange(3, 3)),
                "Objects.equals compares string values and safely handles nulls"
            ),
        )
    }.extract("HEAD")

internal fun Store.launch(
    parent: String,
    id: String,
    skill: String,
    operations: List<String>,
    scope: List<String>
): Delegation {
    val worker = delegate(
        parent,
        id,
        operations,
        scope,
        "\$$skill\nRead /skills/${Registry.skillPath(skill)} and perform the assigned bounded task."
    )
    readTask(worker.token)
    startTask(worker.token, "agent-${randomId()}", skill)
    return worker
}

internal fun Store.batch(): Pair<PlanCreation, Delegation> {
    val created = createPlan(
        "Extract signals from the latest commit",
        listOf(Step("edict-batch-signal-analysis", "Extract latest correction"))
    )
    return created to launch(
        created.token,
        created.plan.tasks.single().id,
        "edict-batch-signal-analysis",
        listOf("inbox.write"),
        listOf("inbox")
    )
}
