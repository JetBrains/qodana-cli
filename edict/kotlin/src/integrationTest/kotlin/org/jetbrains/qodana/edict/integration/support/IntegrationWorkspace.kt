// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration.support

import org.jetbrains.qodana.edict.common.runProcess
import org.jetbrains.qodana.edict.common.sha256
import org.jetbrains.qodana.edict.git.CommitSignalExtractor
import org.jetbrains.qodana.edict.git.GitRepository
import org.jetbrains.qodana.edict.git.SignalFinding
import org.jetbrains.qodana.edict.mcp.McpServer
import org.jetbrains.qodana.edict.model.Signal
import org.jetbrains.qodana.edict.model.SignalLabel
import org.jetbrains.qodana.edict.model.SignalRange
import org.jetbrains.qodana.edict.reviews.ReviewClient
import org.jetbrains.qodana.edict.reviews.ReviewProvider
import org.jetbrains.qodana.edict.runtime.CodexRunner
import org.jetbrains.qodana.edict.store.Store
import java.io.IOException
import java.nio.channels.FileChannel
import java.nio.channels.FileLock
import java.nio.channels.OverlappingFileLockException
import java.nio.file.FileVisitResult
import java.nio.file.Files
import java.nio.file.LinkOption.NOFOLLOW_LINKS
import java.nio.file.Path
import java.nio.file.SimpleFileVisitor
import java.nio.file.StandardOpenOption.CREATE
import java.nio.file.StandardOpenOption.WRITE
import java.nio.file.attribute.BasicFileAttributes
import java.nio.file.attribute.PosixFilePermissions
import kotlin.test.assertEquals
import kotlin.test.assertTrue

internal const val historyProject = "testHistoryFixSignals"
internal const val historyPath = "$historyProject/src/main/java/com/mycompany/app/PollingWaiter.java"
internal const val historyBefore = "b6d8c9ec55d646bd01e0a47b93235376d7b13779"
internal const val historyCommit = "16f44bad3587e0f95ec5ff711ba213820de9ed35"

/** Each test owns a disposable clone; its last output survives until that same test runs again. */
internal class IntegrationWorkspace private constructor(
    val output: Path, val repository: GitRepository, val project: Path,
    val revision: String, private val lock: FileLock, private val channel: FileChannel,
) : AutoCloseable {
    val state = project.resolve(".edict")
    val logs = output.resolve("log/edict")

    fun withCodex(
        prompt: String, provider: ReviewProvider = ReviewClient(), inspectionUrl: String? = null,
        timeoutMinutes: Long = 20, verify: (Store, CodexRunner, String) -> Unit,
    ) {
        Store(state).use { store ->
            val server = McpServer(store, provider = provider, logs = logs)
            server.serveHttp().use { transport ->
                val runtime = CodexRunner(
                    output, project, state, transport.url, agentLogger = server.agents,
                    additionalMcpServers = inspectionUrl?.let { mapOf("inspection" to it) }.orEmpty()
                )
                runtime.prepare()
                runtime.verifySandbox()
                println("Managed run: ${runtime.model}; logs: $logs")
                val result = runtime.run(prompt, timeoutMinutes)
                println(store.redact(result))
                verify(store, runtime, result)
            }
        }
    }

    fun signals(): List<Signal> = CommitSignalExtractor(repository) { commit, repo ->
        check(commit.parentRevision == historyBefore && commit.commitRevision == historyCommit)
        check("Thread.sleep(1_000)" in repo.fileAt(historyBefore, historyPath))
        check("workerFinished.await(1, TimeUnit.SECONDS)" in repo.fileAt(historyCommit, historyPath))
        listOf(
            SignalFinding(
                SignalLabel.POSITIVE,
                historyPath,
                listOf(SignalRange(5, 5)),
                "Sleeping does not reliably coordinate worker completion"
            ),
            SignalFinding(
                SignalLabel.NEGATIVE,
                historyPath,
                listOf(SignalRange(10, 10)),
                "Await a completion signal instead of sleeping to coordinate a worker"
            ),
        )
    }.extract(revision)

    fun assertCheckoutUnchanged() {
        assertEquals(revision, repository.resolve("HEAD"), "Integration changed checkout HEAD")
        assertEquals(
            "",
            repository.git("diff", "--name-only", revision, "--"),
            "Integration modified tracked fixture content"
        )
        val allowed = repository.root.relativize(state).toString().replace('\\', '/') + "/"
        repository.git("ls-files", "--others", "--exclude-standard", "-z").split('\u0000').filter(String::isNotBlank)
            .forEach {
                assertTrue(it.startsWith(allowed), "Integration wrote outside managed state: $it")
            }
    }

    override fun close() {
        try {
            lock.release()
        } finally {
            channel.close()
        }
    }

    companion object {
        fun open(
            outputDirectory: Path, testClass: String, testMethod: String,
            source: String = defaultSource(), revision: String = historyCommit, projectPath: String = historyProject,
        ): IntegrationWorkspace {
            val base = outputDirectory.toAbsolutePath().normalize()
            generateSequence(base) { it.parent }.forEach { require(!Files.isSymbolicLink(it)) { "Refusing to clear integration output through symlink: $it" } }
            require(testClass.matches(Regex("[A-Za-z0-9_.-]+")) && testClass !in listOf(".", ".."))
            val name = testMethod.replace(Regex("[^A-Za-z0-9_-]+"), "-").take(100) + "-" + sha256(testMethod).take(8)
            val output = base.resolve(testClass).resolve(name)
            privateDirectory(base)
            val locks = base.resolve(".locks")
            require(!Files.isSymbolicLink(locks)) { "Integration lock directory must not be a symlink" }
            privateDirectory(locks)
            val lockPath = locks.resolve(sha256("$testClass/$testMethod") + ".lock")
            val channel = FileChannel.open(lockPath, CREATE, WRITE, NOFOLLOW_LINKS)
            var lock: FileLock? = null
            try {
                lock = try {
                    channel.tryLock()
                } catch (_: OverlappingFileLockException) {
                    null
                }
                check(lock != null) { "Integration test is already running: $testClass/$testMethod; use a separate output directory" }
                require(!Files.isSymbolicLink(output.parent) && !Files.isSymbolicLink(output)) { "Refusing to clear redirected integration output: $output" }
                if (Files.exists(output, NOFOLLOW_LINKS)) Files.walkFileTree(
                    output,
                    object : SimpleFileVisitor<Path>() {
                        override fun visitFile(file: Path, attrs: BasicFileAttributes): FileVisitResult {
                            Files.delete(file); return FileVisitResult.CONTINUE
                        }

                        override fun postVisitDirectory(dir: Path, exc: IOException?): FileVisitResult {
                            if (exc != null) throw exc
                            Files.delete(dir); return FileVisitResult.CONTINUE
                        }
                    })
                privateDirectory(output.parent)
                privateDirectory(output)
                val clone = output.resolve("distillery-test")
                runProcess(
                    output,
                    listOf("git", "clone", "--quiet", "--no-local", "--", source, clone.toString()),
                    timeoutSeconds = 180
                )
                val repository = GitRepository(clone)
                val exact = repository.resolve(revision)
                repository.git("checkout", "--quiet", "--detach", exact)
                repository.git("checkout", "--quiet", "-B", "main", exact)
                repository.git("reset", "--hard", exact)
                repository.git("clean", "-ffdx")
                check(
                    repository.git("status", "--porcelain").isEmpty()
                ) { "Distillery clone must be clean before execution" }
                val project = clone.resolve(projectPath).normalize()
                require(project.startsWith(clone) && Files.isDirectory(project) && !Files.isSymbolicLink(project)) { "Missing fixture project: $projectPath" }
                return IntegrationWorkspace(output, repository, project, exact, lock, channel)
            } catch (e: Throwable) {
                lock?.release(); channel.close(); throw e
            }
        }

        private fun privateDirectory(path: Path) {
            Files.createDirectories(path)
            if (Files.getFileStore(path).supportsFileAttributeView("posix"))
                Files.setPosixFilePermissions(path, PosixFilePermissions.fromString("rwx------"))
        }

        private fun defaultSource(): String {
            System.getenv("DISTILLERY_TEST_REPO")?.takeIf(String::isNotBlank)?.let { return it }
            val local = Path.of(System.getProperty("user.home"), "prj/examples/distillery-test")
            return if (Files.exists(local.resolve(".git"))) local.toString() else "ssh://git@git.jetbrains.team/sa/distillery-test.git"
        }
    }
}
