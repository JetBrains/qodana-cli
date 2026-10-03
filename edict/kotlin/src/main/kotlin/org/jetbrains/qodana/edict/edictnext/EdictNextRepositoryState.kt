package org.jetbrains.qodana.edict.edictnext

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.randomId
import org.jetbrains.qodana.edict.common.sha256
import org.jetbrains.qodana.edict.signals.SignalValidation
import org.jetbrains.qodana.edict.skills.managed.Registry
import java.nio.ByteBuffer
import java.nio.channels.FileChannel
import java.nio.file.Files
import java.nio.file.LinkOption.NOFOLLOW_LINKS
import java.nio.file.Path
import java.nio.file.StandardCopyOption
import java.nio.file.StandardOpenOption.CREATE
import java.nio.file.StandardOpenOption.WRITE
import java.nio.file.attribute.PosixFilePermissions

/** Repository snapshot plus the persisted execution state associated with an Edict Next run. */
internal class EdictNextRepositoryState(
  val root: Path,
  val clusters: List<EdictNextStoredCluster>,
  val inboxSignals: List<EdictNextSignal>,
  val filesByRelativePath: Map<String, ByteArray>,
) : AutoCloseable {
  @Serializable
  data class Step(val skill: String, val title: String)

  @Serializable
  data class Task(
    val id: String = randomId(),
    val parentId: String = "",
    val skill: String,
    val title: String,
    val prompt: String = "",
    val status: String = "pending",
    val agentId: String = "",
    val result: String = "",
  )

  @Serializable
  data class Plan(
    val id: String = randomId(),
    val request: String,
    val revision: Int = 0,
    val tasks: List<Task>,
  )

  @Serializable
  data class PlanCreation(val plan: Plan, val token: String)

  @Serializable
  data class Delegation(
    val token: String,
    val taskId: String,
    val skill: String,
    val skillPath: String,
    val prompt: String,
  )

  @Serializable
  data class TaskAssignment(val taskId: String, val skill: String, val skillPath: String, val prompt: String)

  @Serializable
  data class StateArtifact(val path: String, val content: String, val hash: String)

  private data class Capability(
    val skill: String,
    val taskId: String = "",
    val parent: String = "",
    var taskRead: Boolean = false,
  )

  val clustersById: Map<String, EdictNextStoredCluster> = clusters.associateBy(EdictNextStoredCluster::id)
  val inboxSignalsById: Map<String, EdictNextSignal> = inboxSignals.associateBy(EdictNextSignal::id)
  val inboxSignalIds: Set<String> = inboxSignals.mapTo(LinkedHashSet(), EdictNextSignal::id)
  val jvmInboxSignalIds: Set<String> = inboxSignals.filter(EdictNextSignal::isJvmLanguage)
    .mapTo(LinkedHashSet(), EdictNextSignal::id)
  val signalsById: Map<String, EdictNextSignal> = (clusters.flatMap(EdictNextStoredCluster::signals) + inboxSignals)
    .associateBy(EdictNextSignal::id)

  private var lockChannel: FileChannel? = null
  private val grants = mutableMapOf<String, Capability>()
  private val issuedTokens = mutableSetOf<String>()
  private var currentPlan: Plan? = null
  private var managerClaimed = false
  private var closed = false

  fun relativePath(path: Path): String = root.relativize(path).toString()

  fun inspectionCode(clusterId: String): String? = filesByRelativePath[
    "inspections/$clusterId$EDICT_NEXT_INSPECTION_SUFFIX"
  ]?.decodeToString()

  @Synchronized
  fun plan(): Plan? {
    ensureManagementState()
    return currentPlan?.let { EdictNextJson.decodeFromString<Plan>(EdictNextJson.encodeToString(it)) }
  }

  @Synchronized
  fun createPlan(request: String, steps: List<Step>): PlanCreation {
    ensureManagementState()
    check(!managerClaimed) { "edict_plan_create has already succeeded for this server" }
    require(request.isNotBlank() && steps.size in 1..1000) { "A request and 1..1000 steps are required" }
    val manager = Capability(skill = "edict_manager")
    steps.forEach { allowedChild(manager, it.skill, it.title) }
    val existing = currentPlan
    if (existing != null && existing.tasks.any { it.status !in TERMINAL_STATUSES }) {
      require(
        existing.request == request &&
          existing.tasks.filter { it.parentId.isEmpty() }.map { Step(it.skill, it.title) } == steps,
      ) { "Resume unfinished plan with its original request and top-level steps" }
    }
    else {
      save(Plan(request = request, tasks = steps.map { Task(skill = it.skill, title = it.title) }))
    }
    val token = issue(manager)
    managerClaimed = true
    return PlanCreation(checkNotNull(plan()), token)
  }

  @Synchronized
  fun addTask(token: String, skill: String, title: String): Task {
    val capability = authorize(token)
    allowedChild(capability, skill, title)
    val plan = checkNotNull(currentPlan)
    if (capability.skill == "edict-next-cluster-generation" && skill in GENERATION_REVIEWS) {
      require(plan.tasks.count { it.parentId == capability.taskId && it.skill == skill } <= MAX_REPAIR_ITERATIONS) {
        "Three review repair iterations exhausted for $skill; finish this cluster with its validated outcome and remaining findings"
      }
    }
    require(plan.tasks.size < 10_000) { "Task limit reached" }
    return Task(parentId = capability.taskId, skill = skill, title = title).also { task ->
      save(plan.copy(tasks = plan.tasks + task))
    }
  }

  @Synchronized
  fun delegate(token: String, taskId: String, prompt: String): Delegation {
    val parent = authorize(token)
    val task = task(taskId)
    require(task.parentId == parent.taskId) { "Task is not a direct child of this capability" }
    allowedChild(parent, task.skill, task.title)
    require(task.status in listOf("pending", "failed")) { "Task is already delegated or completed" }
    require(prompt.substringBefore('\n') == "\$${task.skill}" && prompt.substringAfter('\n', "").isNotBlank()) {
      "Subagent prompt must start with the exact line \$${task.skill} followed by its skill path and bounded task instructions"
    }
    requireNoTokens(prompt)
    update(task.copy(status = "delegated", agentId = "", result = "", prompt = prompt))
    val secret = issue(Capability(task.skill, taskId, sha256(token)))
    return Delegation(
      secret,
      taskId,
      task.skill,
      Registry.skillPath(task.skill),
      "\$${task.skill}\nBefore loading any skill, call edict_task_get with exactly these arguments:\n" +
        "{\"token\":\"$secret\"}\n" +
        "The token selects your assigned task. taskId is returned by this call; it is not an input argument. " +
        "Read the returned prompt and assigned SKILL.md, then execute it using the managed lifecycle. " +
        "Use only the assigned worker skill; edict_manager is for the root manager.",
    )
  }

  @Synchronized
  fun readTask(token: String): TaskAssignment {
    val capability = lookup(token)
    val task = task(capability.taskId)
    require(task.status in listOf("delegated", "running") && task.prompt.isNotEmpty()) {
      "Only delegated workers can read their assignment"
    }
    capability.taskRead = true
    return TaskAssignment(task.id, task.skill, Registry.skillPath(task.skill), task.prompt)
  }

  @Synchronized
  fun startTask(token: String, agentId: String, skill: String): Plan {
    val capability = lookup(token)
    val task = task(capability.taskId)
    require(task.status == "delegated") { "Only a delegated worker can start its task" }
    require(capability.taskRead) { "Read your assignment with edict_task_get before starting" }
    require(skill == capability.skill) { "Task skill mismatch: expected ${capability.skill}" }
    require(agentId.isNotBlank() && checkNotNull(currentPlan).tasks.none { it.agentId == agentId }) {
      "Each task requires a fresh subagent ID"
    }
    requireNoTokens(agentId)
    update(task.copy(status = "running", agentId = agentId))
    return checkNotNull(plan())
  }

  @Synchronized
  fun finishTask(token: String, status: String, result: String): Plan {
    val capability = authorize(token)
    require(capability.taskId.isNotEmpty()) { "Manager must finish tasks through their workers" }
    require(status in TERMINAL_STATUSES && result.isNotBlank()) { "Requires completed or failed status and a result" }
    requireNoTokens(result)
    val plan = checkNotNull(currentPlan)
    val tasks = plan.tasks.map { task ->
      when {
        task.id == capability.taskId -> task.copy(status = status, result = result)
        descendant(task.id, capability.taskId) -> {
          require(status != "completed" || task.status == "completed") {
            "Complete all subtasks before completing their parent"
          }
          if (task.status == "completed") task else task.copy(status = "failed", result = "Parent task failed: $result")
        }
        else -> task
      }
    }
    save(plan.copy(tasks = tasks))
    revoke(capability.taskId)
    return checkNotNull(plan())
  }

  @Synchronized
  fun cancelTask(token: String, taskId: String, result: String): Plan {
    val capability = authorize(token)
    val task = task(taskId)
    require(task.parentId == capability.taskId) { "Only direct coordinator can cancel a task" }
    require(task.status !in TERMINAL_STATUSES && result.isNotBlank()) {
      "Cancellation requires an unfinished task and a reason"
    }
    requireNoTokens(result)
    val plan = checkNotNull(currentPlan)
    save(plan.copy(tasks = plan.tasks.map { candidate ->
      if (candidate.id == taskId || descendant(candidate.id, taskId) && candidate.status != "completed") {
        candidate.copy(status = "failed", result = result)
      }
      else candidate
    }))
    revoke(taskId)
    return checkNotNull(plan())
  }

  /** Publish a validated extraction result without granting workers direct filesystem write access. */
  @Synchronized
  fun writeSignal(
    token: String,
    path: String,
    content: String,
    expectedHash: String,
    validateEvidence: (EdictNextSignal) -> Unit = {},
  ): StateArtifact {
    val capability = authorize(token)
    require(capability.skill in SIGNAL_WRITERS) { "${capability.skill} cannot publish inbox Signals" }
    require(path.matches(Regex("inbox/s-[0-9a-f]{10}\\.json"))) {
      "Signal writes require an inbox/s-<10 lowercase hex>.json path"
    }
    require(content.toByteArray(Charsets.UTF_8).size <= MAX_PLAN_BYTES) { "Signal exceeds 8 MiB" }
    requireNoTokens(content)
    val signal = SignalValidation.validate(path, content)
    require(capability.skill != "edict-batch-signal-analysis" || signal.source is EdictNextSignalSource.FromCommit) {
      "Commit extraction can publish only FromCommit Signals"
    }
    require(capability.skill != "edict-pr-signal-analysis" || signal.source is EdictNextSignalSource.FromPR) {
      "PR extraction can publish only FromPR Signals"
    }
    validateEvidence(signal)

    val target = safePath(path)
    val existing = if (Files.exists(target, NOFOLLOW_LINKS)) Files.readString(target) else null
    if (existing != null) {
      require(existing == content || expectedHash.isNotEmpty() && sha256(existing) == expectedHash) {
        "Signal already exists with different content or hash"
      }
      if (existing == content) return StateArtifact(path, existing, sha256(existing))
    }
    else {
      require(expectedHash.isEmpty()) { "New Signal requires an empty expectedHash" }
    }
    atomicWrite(path, content)
    return StateArtifact(path, content, sha256(content))
  }

  @Synchronized
  fun redact(text: String): String {
    if (issuedTokens.isEmpty()) return text
    return buildString {
      var copiedUntil = 0
      POSSIBLE_TOKEN.findAll(text).forEach { match ->
        if (sha256(match.groupValues[1]) in issuedTokens) {
          val start = match.range.first
          if (start >= copiedUntil) append(text, copiedUntil, start).append("[REDACTED]")
          copiedUntil = maxOf(copiedUntil, start + 64)
        }
      }
      append(text, copiedUntil, text.length)
    }
  }

  @Synchronized
  fun caller(token: String): String = grants[sha256(token)]?.let {
    "${it.skill}/${it.taskId.ifEmpty { "manager" }}"
  } ?: "anonymous"

  @Synchronized
  fun requireSkill(token: String, allowedSkills: Set<String>): String {
    val capability = authorize(token)
    require(capability.skill in allowedSkills) {
      "${capability.skill} is not allowed to perform this managed mutation"
    }
    return capability.skill
  }

  @Synchronized
  fun requireTokenFree(content: String) {
    requireNoTokens(content)
  }

  @Synchronized
  internal fun prAnalysisOwner(token: String, coordinator: Boolean): String {
    val capability = authorize(token)
    if (capability.skill == "edict-pr-signal-analysis") return capability.taskId
    if (!coordinator && capability.skill == "edict-signal-analysis") {
      val parent = task(task(capability.taskId).parentId)
      if (parent.skill == "edict-pr-signal-analysis") return parent.id
    }
    error("PR analysis requires a running PR-analysis task${if (coordinator) "" else " or its signal-analysis worker"}")
  }

  @Synchronized
  internal fun isPrAnalysisCoordinator(token: String): Boolean =
    authorize(token).skill == "edict-pr-signal-analysis"

  @Synchronized
  internal fun inboxSignalHash(id: String): String? {
    require(id.matches(Regex("s-[0-9a-f]{10}"))) { "Invalid Signal ID" }
    val path = safePath("inbox/$id.json")
    return if (Files.exists(path, NOFOLLOW_LINKS)) sha256(Files.readString(path)) else null
  }

  @Synchronized
  override fun close() {
    if (closed) return
    closed = true
    grants.clear()
    lockChannel?.close()
    lockChannel = null
  }

  private fun ensureManagementState() {
    check(!closed) { "Repository state is closed" }
    if (lockChannel != null) return
    Files.createDirectories(root)
    require(!Files.isSymbolicLink(root)) { "State root must not be a symlink" }
    val channel = FileChannel.open(safePath(LOCK_FILE), CREATE, WRITE, NOFOLLOW_LINKS)
    try {
      check(channel.tryLock() != null) { "State is already owned by another Edict management service" }
      lockChannel = channel
      restore()
    }
    catch (e: Exception) {
      channel.close()
      throw e
    }
  }

  private fun authorize(token: String): Capability = lookup(token).also { capability ->
    require(capability.taskId.isEmpty() || task(capability.taskId).status == "running") {
      "Worker must start its delegated task first"
    }
  }

  private fun lookup(token: String): Capability {
    ensureManagementState()
    val capability = grants[sha256(token)] ?: error("Invalid or revoked capability")
    check(capability.parent.isEmpty() || capability.parent in grants) { "Parent capability is revoked" }
    return capability
  }

  private fun task(id: String): Task = currentPlan?.tasks?.firstOrNull { it.id == id } ?: error("Unknown task")

  private fun issue(capability: Capability): String = randomId().also { token ->
    val digest = sha256(token)
    issuedTokens += digest
    grants[digest] = capability
  }

  private fun requireNoTokens(text: String) {
    require(redact(text) == text) { "Persisted content must not contain capability tokens" }
  }

  private fun allowedChild(capability: Capability, skill: String, title: String) {
    require(title.isNotBlank()) { "Task title is required" }
    require(skill in Registry[capability.skill].delegates) {
      "${capability.skill} cannot call managed skill $skill"
    }
    requireNoTokens(title)
  }

  private fun descendant(id: String, ancestor: String): Boolean {
    var task = currentPlan?.tasks?.firstOrNull { it.id == id }
    while (task != null && task.parentId.isNotEmpty()) {
      if (task.parentId == ancestor) return true
      task = task(task.parentId)
    }
    return false
  }

  private fun revoke(id: String) {
    grants.entries.removeIf { it.value.taskId == id || descendant(it.value.taskId, id) }
  }

  private fun update(task: Task) {
    val plan = checkNotNull(currentPlan)
    save(plan.copy(tasks = plan.tasks.map { if (it.id == task.id) task else it }))
  }

  private fun save(plan: Plan) {
    ensureManagementState()
    val updated = plan.copy(revision = plan.revision + 1)
    val content = EdictNextJson.encodeToString(updated) + "\n"
    require(content.toByteArray(Charsets.UTF_8).size <= MAX_PLAN_BYTES) { "Execution plan exceeds 8 MiB" }
    atomicWrite("plans/${updated.id}.json", content)
    if (currentPlan?.id != updated.id) atomicWrite(CURRENT_PLAN_FILE, updated.id)
    currentPlan = updated
  }

  private fun restore() {
    val pointer = safePath(CURRENT_PLAN_FILE)
    if (!Files.exists(pointer, NOFOLLOW_LINKS)) return
    val id = readText(CURRENT_PLAN_FILE)
    require(IDENTIFIER.matches(id)) { "Invalid active plan ID" }
    val persisted = EdictNextJson.parseToJsonElement(readText("plans/$id.json")).jsonObject
    require(listOf("id", "request", "revision", "tasks").all { it in persisted }) { "Incomplete saved plan" }
    persisted.getValue("tasks").jsonArray.forEach { task ->
      require(listOf("id", "skill", "title", "status").all { it in task.jsonObject }) {
        "Incomplete saved task"
      }
    }
    val plan = EdictNextJson.decodeFromJsonElement<Plan>(persisted)
    require(plan.id == id && plan.revision > 0) { "Invalid saved plan identity" }
    val seen = mutableSetOf<String>()
    plan.tasks.forEach { task ->
      require(IDENTIFIER.matches(task.id) && task.id !in seen && (task.parentId.isEmpty() || task.parentId in seen)) {
        "Invalid saved task graph"
      }
      seen += task.id
      Registry[task.skill]
      require(task.status in TASK_STATUSES) { "Invalid saved task status" }
    }
    currentPlan = plan
    if (plan.tasks.any { it.status in INTERRUPTED_STATUSES }) {
      save(plan.copy(tasks = plan.tasks.map { task ->
        if (task.status in INTERRUPTED_STATUSES) {
          task.copy(
            status = "pending",
            agentId = "",
            result = "Interrupted by server restart; delegate to a fresh worker.",
          )
        }
        else task
      }))
    }
  }

  private fun readText(relative: String): String {
    val bytes = Files.newInputStream(safePath(relative), NOFOLLOW_LINKS).use { it.readNBytes(MAX_PLAN_BYTES + 1) }
    require(bytes.size <= MAX_PLAN_BYTES) { "Execution plan exceeds 8 MiB" }
    return bytes.toString(Charsets.UTF_8)
  }

  private fun safePath(relative: String): Path {
    check(!closed) { "Repository state is closed" }
    require(relative.isNotBlank() && !Path.of(relative).isAbsolute) { "Invalid state path" }
    var path = root
    relative.split('/').forEach { part ->
      require(part.isNotBlank() && part != "." && part != "..") { "Invalid state path" }
      path = path.resolve(part)
      require(!Files.isSymbolicLink(path)) { "Symlinks are forbidden: $relative" }
    }
    return path.normalize().also { require(it.startsWith(root)) { "State path escapes its root" } }
  }

  private fun atomicWrite(relative: String, content: String) {
    val target = safePath(relative)
    Files.createDirectories(target.parent)
    val attributes = if (Files.getFileStore(root).supportsFileAttributeView("posix")) {
      arrayOf(PosixFilePermissions.asFileAttribute(PosixFilePermissions.fromString("rw-------")))
    }
    else emptyArray()
    val temporary = Files.createTempFile(target.parent, ".edict-tmp-", "", *attributes)
    try {
      FileChannel.open(temporary, WRITE).use { channel ->
        val buffer = ByteBuffer.wrap(content.toByteArray(Charsets.UTF_8))
        while (buffer.hasRemaining()) channel.write(buffer)
        channel.force(true)
      }
      Files.move(temporary, target, StandardCopyOption.ATOMIC_MOVE, StandardCopyOption.REPLACE_EXISTING)
    }
    finally {
      Files.deleteIfExists(temporary)
    }
  }

  companion object {
    private const val MAX_PLAN_BYTES = 8 * 1024 * 1024
    private const val MAX_REPAIR_ITERATIONS = 3
    private const val LOCK_FILE = ".edict-mcp.lock"
    private const val CURRENT_PLAN_FILE = ".edict-mcp-current"
    private val GENERATION_REVIEWS = setOf(
      "edict-next-inspection-code-review",
      "edict-next-weak-signal-review",
    )
    private val SIGNAL_WRITERS = setOf("edict-batch-signal-analysis", "edict-pr-signal-analysis")
    private val TERMINAL_STATUSES = setOf("completed", "failed")
    private val INTERRUPTED_STATUSES = setOf("delegated", "running")
    private val TASK_STATUSES = setOf("pending", "delegated", "running", "completed", "failed")
    private val POSSIBLE_TOKEN = Regex("(?=([0-9a-f]{64}))")
    private val IDENTIFIER = Regex("[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}")

    fun open(root: Path): EdictNextRepositoryState = EdictNextRepositoryState(
      root = root.toAbsolutePath().normalize(),
      clusters = emptyList(),
      inboxSignals = emptyList(),
      filesByRelativePath = emptyMap(),
    ).also(EdictNextRepositoryState::ensureManagementState)
  }
}
