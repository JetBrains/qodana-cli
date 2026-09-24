// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.store

import java.nio.ByteBuffer
import java.nio.channels.FileChannel
import java.nio.file.*
import java.nio.file.LinkOption.NOFOLLOW_LINKS
import java.nio.file.StandardOpenOption.*
import java.nio.file.attribute.PosixFilePermissions
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.common.randomId
import org.jetbrains.qodana.edict.common.sha256
import org.jetbrains.qodana.edict.model.Delegation
import org.jetbrains.qodana.edict.model.Plan
import org.jetbrains.qodana.edict.model.PlanCreation
import org.jetbrains.qodana.edict.model.StateFile
import org.jetbrains.qodana.edict.model.Step
import org.jetbrains.qodana.edict.model.Task
import org.jetbrains.qodana.edict.model.TaskAssignment
import org.jetbrains.qodana.edict.reviews.PrAnalysis
import org.jetbrains.qodana.edict.signals.SignalValidation
import org.jetbrains.qodana.edict.skills.Registry

internal const val MAX_ARTIFACT_BYTES = 8 * 1024 * 1024
// Look ahead so a hexadecimal prefix cannot hide a token inside a longer run.
private val possibleToken = Regex("(?=([0-9a-f]{64}))")

internal data class Capability(
    val skill: String, val taskId: String = "", val parent: String = "",
    val operations: List<String>, val scope: List<String>, var taskRead: Boolean = false,
)

/** The trusted host owns this root and must deny agents direct writes, including directory replacement. */
class Store(directory: Path) : AutoCloseable {
    val root: Path = directory.toAbsolutePath().normalize()
    private val lockChannel: FileChannel
    private val grants = mutableMapOf<String, Capability>()
    private val issuedTokens = mutableSetOf<String>()
    private var current: Plan? = null
    private var managerClaimed = false
    private var closed = false
    internal var prAnalysis: PrAnalysis? = null

    init {
        Files.createDirectories(root)
        require(!Files.isSymbolicLink(root)) { "State root must not be a symlink" }
        lockChannel = FileChannel.open(safePath(".edict-mcp.lock"), CREATE, WRITE, NOFOLLOW_LINKS)
        try {
            check(lockChannel.tryLock() != null) { "State is already owned by another edict-mcp server" }
            restore()
        } catch (e: Exception) { lockChannel.close(); throw e }
    }

    @Synchronized override fun close() {
        if (closed) return
        closed = true
        grants.clear()
        lockChannel.close()
    }

    @Synchronized fun plan(): Plan? = current?.let { json.decodeFromString<Plan>(json.encodeToString(it)) }

    @Synchronized fun createPlan(request: String, steps: List<Step>): PlanCreation {
        check(!closed) { "Store is closed" }
        check(!managerClaimed) { "edict_plan_create has already succeeded for this server" }
        require(request.isNotBlank() && steps.size in 1..1000) { "A request and 1..1000 steps are required" }
        val manager = Capability("edict_manager", operations = Registry["edict_manager"].operations, scope = listOf("inbox", "clusters", "inspections"))
        steps.forEach { allowedChild(manager, it.skill, it.title) }
        val existing = current
        if (existing != null && existing.tasks.any { it.status !in listOf("completed", "failed") }) {
            require(existing.request == request && existing.tasks.filter { it.parentId.isEmpty() }.map { Step(it.skill, it.title) } == steps) {
                "Resume unfinished plan with its original request and top-level steps"
            }
        } else save(Plan(request = request, tasks = steps.map { Task(skill = it.skill, title = it.title) }))
        val token = issue(manager)
        managerClaimed = true
        return PlanCreation(plan()!!, token)
    }

    @Synchronized fun addTask(token: String, skill: String, title: String): Task {
        val c = authorize(token)
        allowedChild(c, skill, title)
        val p = checkNotNull(current)
        require(p.tasks.size < 10000) { "Task limit reached" }
        val task = Task(parentId = c.taskId, skill = skill, title = title)
        save(p.copy(tasks = p.tasks + task))
        return task
    }

    @Synchronized fun delegate(token: String, taskId: String, operations: List<String>, scope: List<String>, prompt: String): Delegation {
        val c = authorize(token)
        val task = task(taskId)
        require(task.parentId == c.taskId) { "Task is not a direct child of this capability" }
        allowedChild(c, task.skill, task.title)
        require(task.status in listOf("pending", "failed")) { "Task is already delegated or completed" }
        require(operations.all { it in c.operations && it in Registry[task.skill].operations }) { "Cannot widen delegated operations" }
        scope.forEach { validArtifactPath(it); require(covered(c.scope, it)) { "Cannot widen scope to $it" } }
        require(operations.isEmpty() || scope.isNotEmpty()) { "Mutation operations require explicit scope" }
        require(prompt.substringBefore('\n') == "\$${task.skill}" && prompt.substringAfter('\n', "").isNotBlank()) {
            "Subagent prompt must start with the exact line \$${task.skill} followed by its skill path and bounded task instructions"
        }
        requireNoTokens(prompt)
        update(task.copy(status = "delegated", agentId = "", result = "", operations = operations.toList(), scope = scope.toList(), prompt = prompt))
        val secret = issue(Capability(task.skill, taskId, sha256(token), operations.toList(), scope.toList()))
        return Delegation(secret, taskId, task.skill, Registry.skillPath(task.skill),
            "\$${task.skill}\nBefore loading any skill, call edict_task_get with exactly these arguments:\n" +
                "{\"token\":\"$secret\"}\n" +
                "The token selects your assigned task. taskId is returned by this call; it is not an input argument. " +
                "Read the returned prompt and assigned SKILL.md, then execute it using the managed lifecycle. Use only the assigned worker skill; edict_manager is for the root manager.")
    }

    @Synchronized fun readTask(token: String): TaskAssignment {
        val c = lookup(token)
        val task = task(c.taskId)
        require(task.status in listOf("delegated", "running") && task.prompt.isNotEmpty()) { "Only delegated workers can read their assignment" }
        c.taskRead = true
        return TaskAssignment(task.id, task.skill, Registry.skillPath(task.skill), task.prompt)
    }

    @Synchronized fun startTask(token: String, agentId: String, skill: String): Plan {
        val c = lookup(token)
        val task = task(c.taskId)
        require(task.status == "delegated") { "Only a delegated worker can start its task" }
        require(c.taskRead) { "Read your assignment with edict_task_get before starting" }
        require(skill == c.skill) { "Task skill mismatch: expected ${c.skill}" }
        require(agentId.isNotBlank() && current!!.tasks.none { it.agentId == agentId }) { "Each task requires a fresh subagent ID" }
        requireNoTokens(agentId)
        update(task.copy(status = "running", agentId = agentId))
        return plan()!!
    }

    @Synchronized fun finishTask(token: String, status: String, result: String): Plan {
        val c = authorize(token)
        require(c.taskId.isNotEmpty()) { "Manager must finish tasks through their workers" }
        require(status in listOf("completed", "failed") && result.isNotBlank()) { "Requires completed or failed status and a result" }
        requireNoTokens(result)
        if (status == "completed" && c.skill == "edict-pr-signal-analysis") checkNotNull(prAnalysis).complete(c.taskId)
        val p = current!!
        val tasks = p.tasks.map { t ->
            when {
                t.id == c.taskId -> t.copy(status = status, result = result)
                descendant(t.id, c.taskId) -> {
                    require(status != "completed" || t.status == "completed") { "Complete all subtasks before completing their parent" }
                    if (t.status != "completed") t.copy(status = "failed", result = "Parent task failed: $result") else t
                }
                else -> t
            }
        }
        save(p.copy(tasks = tasks))
        revoke(c.taskId)
        return plan()!!
    }

    @Synchronized fun cancelTask(token: String, taskId: String, result: String): Plan {
        val c = authorize(token)
        val task = task(taskId)
        require(task.parentId == c.taskId) { "Only direct coordinator can cancel a task" }
        require(task.status !in listOf("completed", "failed") && result.isNotBlank()) { "Cancellation requires an unfinished task and a reason" }
        requireNoTokens(result)
        save(current!!.copy(tasks = current!!.tasks.map { t ->
            if (t.id == taskId || descendant(t.id, taskId) && t.status != "completed") t.copy(status = "failed", result = result) else t
        }))
        revoke(taskId)
        return plan()!!
    }

    @Synchronized fun read(name: String): StateFile {
        require(readable(name)) { "Not a persisted Edict artifact" }
        return readFile(name)
    }

    @Synchronized fun list(prefix: String = ""): List<String> {
        if (prefix.isNotEmpty()) validArtifactPath(prefix)
        return listOf("inbox", "clusters", "inspections", "plans").flatMap { directory ->
            if (prefix.isNotEmpty() && !covered(listOf(directory), prefix) && !covered(listOf(prefix), directory)) return@flatMap emptyList()
            val path = safePath(directory)
            if (!Files.exists(path, NOFOLLOW_LINKS)) emptyList() else Files.walk(path).use { paths ->
                paths.map { p ->
                    val relative = root.relativize(p).joinToString("/")
                    safePath(relative)
                    relative
                }.filter { readable(it) && (prefix.isEmpty() || covered(listOf(prefix), it)) && Files.isRegularFile(root.resolve(it), NOFOLLOW_LINKS) }.toList()
            }
        }.sorted()
    }

    @Synchronized fun write(token: String, name: String, content: String, expectedHash: String): StateFile {
        val c = authorizeWrite(token, name, false)
        require(content.toByteArray(Charsets.UTF_8).size <= MAX_ARTIFACT_BYTES) { "Artifact exceeds 8 MiB" }
        requireNoTokens(content)
        val parsed = if (name.endsWith(".json")) json.parseToJsonElement(content) else null
        val old = compare(name, expectedHash)
        if (name.endsWith("/description.json")) require(parsed?.jsonObject?.get("id")?.jsonPrimitive?.content == name.split('/')[1]) {
            "Cluster description ID must match its directory"
        }
        val op = operation(name)
        if (c.skill == "edict-code-example" && op == "cluster.signal.write") {
            require(old != null) { "Example assignment requires an existing signal" }
            val before = json.parseToJsonElement(old.content).jsonObject
            val after = parsed!!.jsonObject
            require(identifier.matches(after["syntheticExampleId"]?.jsonPrimitive?.content.orEmpty())) { "syntheticExampleId must name an example" }
            require(before - "syntheticExampleId" == after - "syntheticExampleId") { "Example workers may change only syntheticExampleId" }
        }
        if (op == "inbox.write" || op == "cluster.signal.write") {
            val signal = SignalValidation.validate(name, content)
            if (op == "inbox.write") {
                require(c.skill != "edict-batch-signal-analysis" || signal.source.type != "FromPR") { "PR signals require edict-pr-signal-analysis" }
                if (c.skill == "edict-pr-signal-analysis") checkNotNull(prAnalysis).validateWrite(c.taskId, signal.id, content)
            }
        }
        atomicWrite(name, content)
        return StateFile(name, content)
    }

    @Synchronized fun delete(token: String, name: String, expectedHash: String) {
        val c = authorizeWrite(token, name, true)
        require(c.skill != "edict-code-example" || operation(name) != "cluster.signal.write") { "Example workers cannot delete signals" }
        require(expectedHash.isNotEmpty()) { "Deletion requires existing artifact hash" }
        compare(name, expectedHash)
        Files.delete(safePath(name))
    }

    @Synchronized fun redact(text: String): String {
        if (issuedTokens.isEmpty()) return text
        return buildString {
            var copiedUntil = 0
            possibleToken.findAll(text).forEach { match ->
                if (sha256(match.groupValues[1]) in issuedTokens) {
                    val start = match.range.first
                    if (start >= copiedUntil) append(text, copiedUntil, start).append("[REDACTED]")
                    copiedUntil = maxOf(copiedUntil, start + 64)
                }
            }
            append(text, copiedUntil, text.length)
        }
    }
    @Synchronized internal fun caller(token: String): String = grants[sha256(token)]?.let { "${it.skill}/${it.taskId.ifEmpty { "manager" }}" } ?: "anonymous"

    internal fun authorize(token: String): Capability = lookup(token).also { c ->
        require(c.taskId.isEmpty() || task(c.taskId).status == "running") { "Worker must start its delegated task first" }
    }
    internal fun task(id: String): Task = current?.tasks?.firstOrNull { it.id == id } ?: error("Unknown task")
    private fun lookup(token: String): Capability {
        check(!closed) { "Store is closed" }
        val c = grants[sha256(token)] ?: error("Invalid or revoked capability")
        check(c.parent.isEmpty() || c.parent in grants) { "Parent capability is revoked" }
        return c
    }
    private fun issue(c: Capability): String = randomId().also { val digest = sha256(it); issuedTokens.add(digest); grants[digest] = c }
    private fun requireNoTokens(text: String) = require(redact(text) == text) { "Persisted content must not contain capability tokens" }
    private fun allowedChild(c: Capability, skill: String, title: String) {
        require(title.isNotBlank()) { "Task title is required" }
        require(skill in Registry[c.skill].delegates) { "${c.skill} cannot call managed skill $skill" }
        requireNoTokens(title)
    }
    private fun descendant(id: String, ancestor: String): Boolean {
        var t = current?.tasks?.firstOrNull { it.id == id }
        while (t != null && t.parentId.isNotEmpty()) {
            if (t.parentId == ancestor) return true
            t = task(t.parentId)
        }
        return false
    }
    private fun revoke(id: String) { grants.entries.removeIf { it.value.taskId == id || descendant(it.value.taskId, id) } }
    private fun update(task: Task) { save(current!!.copy(tasks = current!!.tasks.map { if (it.id == task.id) task else it })) }
    private fun authorizeWrite(token: String, name: String, deleting: Boolean): Capability {
        val c = authorize(token)
        val op = operation(name).let { if (deleting && it == "inbox.write") "inbox.delete" else it }
        require(op.isNotEmpty() && op in c.operations && op in Registry[c.skill].writes && covered(c.scope, name)) { "${c.skill} cannot modify $name" }
        safePath(name)
        return c
    }
    private fun compare(name: String, expected: String): StateFile? {
        val old = try { readFile(name) } catch (_: NoSuchFileException) { null }
        require((old?.hash ?: "") == expected) { "Artifact hash conflict; read it again before modifying" }
        return old
    }
    private fun readFile(name: String): StateFile {
        val content = Files.newInputStream(safePath(name), NOFOLLOW_LINKS).use { it.readNBytes(MAX_ARTIFACT_BYTES + 1) }
        require(content.size <= MAX_ARTIFACT_BYTES) { "Artifact exceeds 8 MiB" }
        return StateFile(name, content.toString(Charsets.UTF_8))
    }
    private fun safePath(name: String): Path {
        check(!closed) { "Store is closed" }
        validArtifactPath(name)
        var path = root
        for (part in name.split('/')) {
            path = path.resolve(part)
            require(!Files.isSymbolicLink(path)) { "Symlinks are forbidden: $name" }
            if (Files.exists(path, NOFOLLOW_LINKS)) require(Files.isDirectory(path, NOFOLLOW_LINKS) || Files.isRegularFile(path, NOFOLLOW_LINKS)) { "Not a regular artifact path: $name" }
        }
        return path
    }
    private fun atomicWrite(name: String, content: String) {
        val target = safePath(name)
        Files.createDirectories(target.parent)
        val attributes = if (Files.getFileStore(root).supportsFileAttributeView("posix")) arrayOf(PosixFilePermissions.asFileAttribute(PosixFilePermissions.fromString("rw-------"))) else emptyArray()
        val temporary = Files.createTempFile(target.parent, ".edict-tmp-", "", *attributes)
        try {
            FileChannel.open(temporary, WRITE).use { channel ->
                val buffer = ByteBuffer.wrap(content.toByteArray(Charsets.UTF_8))
                while (buffer.hasRemaining()) channel.write(buffer)
                channel.force(true)
            }
            Files.move(temporary, safePath(name), StandardCopyOption.ATOMIC_MOVE, StandardCopyOption.REPLACE_EXISTING)
        } finally { Files.deleteIfExists(temporary) }
    }
    private fun save(plan: Plan) {
        val updated = plan.copy(revision = plan.revision + 1)
        val content = json.encodeToString(updated) + "\n"
        require(content.toByteArray(Charsets.UTF_8).size <= MAX_ARTIFACT_BYTES) { "Execution plan exceeds 8 MiB" }
        atomicWrite("plans/${updated.id}.json", content)
        if (current?.id != updated.id) atomicWrite(".edict-mcp-current", updated.id)
        current = updated
    }
    private fun restore() {
        val pointer = safePath(".edict-mcp-current")
        if (!Files.exists(pointer, NOFOLLOW_LINKS)) return
        val id = readFile(".edict-mcp-current").content
        require(identifier.matches(id)) { "Invalid active plan ID" }
        val persisted = json.parseToJsonElement(readFile("plans/$id.json").content).jsonObject
        require(listOf("id", "request", "revision", "tasks").all { it in persisted }) { "Incomplete saved plan" }
        persisted.getValue("tasks").jsonArray.forEach { task ->
            require(listOf("id", "skill", "title", "status").all { it in task.jsonObject }) { "Incomplete saved task" }
        }
        val p = json.decodeFromJsonElement<Plan>(persisted)
        require(p.id == id && p.revision > 0) { "Invalid saved plan identity" }
        val seen = mutableSetOf<String>()
        p.tasks.forEach { task ->
            require(identifier.matches(task.id) && task.id !in seen && (task.parentId.isEmpty() || task.parentId in seen)) { "Invalid saved task graph" }
            seen.add(task.id)
            Registry[task.skill]
            require(task.status in listOf("pending", "delegated", "running", "completed", "failed")) { "Invalid saved task status" }
        }
        current = p
        if (p.tasks.any { it.status in listOf("running", "delegated") }) save(p.copy(tasks = p.tasks.map { t ->
            if (t.status in listOf("running", "delegated")) t.copy(status = "pending", agentId = "", result = "Interrupted by server restart; delegate to a fresh worker.") else t
        }))
    }
}
