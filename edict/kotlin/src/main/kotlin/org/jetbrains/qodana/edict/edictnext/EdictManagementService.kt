package org.jetbrains.qodana.edict.edictnext

import io.modelcontextprotocol.kotlin.sdk.server.Server
import io.modelcontextprotocol.kotlin.sdk.types.CallToolResult
import io.modelcontextprotocol.kotlin.sdk.types.TextContent
import io.modelcontextprotocol.kotlin.sdk.types.ToolAnnotations
import io.modelcontextprotocol.kotlin.sdk.types.ToolSchema
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.encodeToJsonElement
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonObject
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.logging.AgentLogger
import org.jetbrains.qodana.edict.logging.TaskLifecycleLogger
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Plan
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Step
import org.jetbrains.qodana.edict.skills.managed.Registry
import java.io.PrintWriter
import java.nio.file.Files
import java.nio.file.Path
import java.time.Instant
import java.util.concurrent.ConcurrentHashMap

/** Owns managed execution state and exposes its plan/task lifecycle as MCP tools. */
internal class EdictManagementService(
  private val store: EdictNextRepositoryState,
  private val logs: Path? = null,
  taskOutput: PrintWriter = PrintWriter(System.err, true),
) {
  companion object {
    const val INSTRUCTIONS = "Managed Edict state and execution plans. Root requests enter through edict_manager. " +
      "Each task must execute in a fresh native subagent with its delegated token and no inherited conversation. " +
      "edict_delegate stores full task instructions and returns a short launch prompt: pass that prompt to spawn_agent. " +
      "Workers call edict_task_get, read the assigned SKILL.md, then edict_task_start with their actual agentId and registry skill. " +
      "Include your own token in calls. Never persist tokens or put them in results. Reads are public. " +
      "Only the first edict_plan_create claims the manager capability. This server does not compile or run inspections."
  }

  private data class Invocation(
    val required: List<String>,
    val properties: Set<String>,
    val invoke: (JsonObject) -> JsonElement,
  )

  val agents: AgentLogger? = logs?.let { AgentLogger(store, it) }

  private val taskLogger = TaskLifecycleLogger(store, logs, taskOutput)
  private val invocations = ConcurrentHashMap<String, Invocation>()

  fun registerTools(server: Server) {
    val string = jsonType("string")

    fun properties(vararg names: String): Map<String, JsonElement> = names.associateWith { string }

    fun tool(
      name: String,
      description: String,
      readOnly: Boolean = false,
      required: List<String> = emptyList(),
      properties: Map<String, JsonElement> = emptyMap(),
      invoke: (JsonObject) -> JsonElement,
    ) {
      val allProperties = properties + ("token" to string)
      invocations[name] = Invocation(required, allProperties.keys, invoke)
      server.addTool(
        name = name,
        description = description,
        inputSchema = ToolSchema(
          properties = JsonObject(allProperties),
          required = required,
        ),
        toolAnnotations = ToolAnnotations(readOnlyHint = readOnly),
      ) { request ->
        call(name, request.arguments ?: JsonObject(emptyMap())).toToolResult()
      }
    }

    tool(
      name = "edict_registry",
      description = "Read immutable managed skill policy and permitted call graph.",
      readOnly = true,
    ) {
      buildJsonObject { put("skills", EdictNextJson.encodeToJsonElement(Registry.policies)) }
    }

    tool(
      name = "edict_plan_get",
      description = "Read current execution plan and worker results.",
      readOnly = true,
    ) {
      buildJsonObject { put("plan", EdictNextJson.encodeToJsonElement(store.plan())) }
    }

    val steps = buildJsonObject {
      put("type", "array")
      putJsonObject("items") {
        put("type", "object")
        put("properties", JsonObject(properties("skill", "title")))
        put("required", JsonArray(listOf("skill", "title").map(::JsonPrimitive)))
        put("additionalProperties", false)
      }
    }
    tool(
      name = "edict_plan_create",
      description = "Create or resume an execution plan. First successful call returns manager token; no token needed. Supply original request and ordered top-level steps when resuming.",
      required = listOf("request", "steps"),
      properties = properties("request") + ("steps" to steps),
    ) { arguments ->
      EdictNextJson.encodeToJsonElement(
        store.createPlan(
          arguments.requireString("request"),
          wireJson.decodeFromJsonElement<List<Step>>(arguments.getValue("steps")),
        ),
      )
    }

    tool(
      name = "edict_task_add",
      description = "Add a direct child task allowed by your registered skill.",
      required = listOf("token", "skill", "title"),
      properties = properties("skill", "title"),
    ) { arguments ->
      EdictNextJson.encodeToJsonElement(
        store.addTask(
          arguments.requireString("token"),
          arguments.requireString("skill"),
          arguments.requireString("title"),
        ),
      )
    }

    tool(
      name = "edict_delegate",
      description = "Delegate a registered direct child task. Prompt must start with the exact line \$<registry skill>, then absolute SKILL.md path and bounded token-free instructions. Pass only returned short prompt to native spawn_agent without inherited conversation.",
      required = listOf("token", "taskId", "prompt"),
      properties = properties("taskId", "prompt"),
    ) { arguments ->
      EdictNextJson.encodeToJsonElement(
        store.delegate(
          arguments.requireString("token"),
          arguments.requireString("taskId"),
          arguments.requireString("prompt"),
        ),
      )
    }

    tool(
      name = "edict_task_get",
      description = "Fetch authoritative assignment using only your token; taskId is a returned field, not an input. Read returned prompt and assigned SKILL.md before starting. Required on every delegation including retry.",
      readOnly = true,
      required = listOf("token"),
    ) { arguments ->
      EdictNextJson.encodeToJsonElement(store.readTask(arguments.requireString("token")))
    }

    tool(
      name = "edict_task_start",
      description = "Start after fetching assignment and reading skill. Use assigned registry skill and actual native subagent ID.",
      required = listOf("token", "agentId", "skill"),
      properties = properties("agentId", "skill"),
    ) { arguments ->
      lifecycle {
        store.startTask(
          arguments.requireString("token"),
          arguments.requireString("agentId"),
          arguments.requireString("skill"),
        )
      }
    }

    tool(
      name = "edict_task_finish",
      description = "Finish with status completed or failed and token-free result. All children must complete first. Revokes task and descendant capabilities.",
      required = listOf("token", "status", "result"),
      properties = properties("status", "result"),
    ) { arguments ->
      lifecycle {
        store.finishTask(
          arguments.requireString("token"),
          arguments.requireString("status"),
          arguments.requireString("result"),
        )
      }
    }

    tool(
      name = "edict_task_cancel",
      description = "Fail a lost direct child and revoke its descendants. A coordinator cannot report success for workers.",
      required = listOf("token", "taskId", "result"),
      properties = properties("taskId", "result"),
    ) { arguments ->
      lifecycle {
        store.cancelTask(
          arguments.requireString("token"),
          arguments.requireString("taskId"),
          arguments.requireString("result"),
        )
      }
    }

    tool(
      name = "edict_state_write",
      description = "Publish one validated extraction Signal. path must be inbox/<stable-id>.json; expectedHash is empty for creation or the exact current SHA-256 for replacement.",
      required = listOf("token", "path", "content", "expectedHash"),
      properties = properties("path", "content", "expectedHash"),
    ) { arguments ->
      EdictNextJson.encodeToJsonElement(
        store.writeSignal(
          arguments.requireString("token"),
          arguments.requireString("path"),
          arguments.requireString("content"),
          arguments.requireString("expectedHash"),
        ),
      )
    }
  }

  internal fun call(name: String, arguments: JsonObject): JsonObject {
    val invocation = checkNotNull(invocations[name]) { "Unknown Edict management tool '$name'" }
    val caller = store.caller(arguments.text("token"))
    val result = try {
      require(arguments.keys.all { it in invocation.properties }) { "Unknown tool argument" }
      require(invocation.required.all { it in arguments }) { "Missing required tool argument" }
      invocation.invoke(arguments).successResult()
    }
    catch (e: Exception) {
      errorResult(store.redact(e.message ?: e.javaClass.simpleName))
    }
    log(caller, name, arguments, result)
    return result
  }

  internal fun requireSkill(token: String, vararg allowedSkills: String) {
    store.requireSkill(token, allowedSkills.toSet())
  }

  internal fun requireTokenFree(content: String) {
    store.requireTokenFree(content)
  }

  private fun lifecycle(change: () -> Plan): JsonElement = synchronized(store) {
    val before = store.plan()
    val after = change()
    taskLogger.record(before, after)
    EdictNextJson.encodeToJsonElement(after)
  }

  @Synchronized
  private fun log(caller: String, name: String, arguments: JsonObject, response: JsonObject) {
    val directory = logs ?: return
    Files.createDirectories(directory)
    val prefix = "${Instant.now()} [$caller] $name"
    val summary = "$prefix ${if (response.flag("isError") == true) "failed" else "ok"}\n"
    val sanitized = JsonObject(
      arguments.mapValues { (key, value) -> if (key == "token") JsonPrimitive("[REDACTED]") else value },
    )
    Files.writeString(
      directory.resolve("edict-mcp.log"),
      store.redact(summary),
      java.nio.file.StandardOpenOption.CREATE,
      java.nio.file.StandardOpenOption.APPEND,
    )
    Files.writeString(
      directory.resolve("edict-mcp-system.log"),
      store.redact("$prefix ${wireJson.encodeToString(sanitized)} => ${wireJson.encodeToString(response)}\n"),
      java.nio.file.StandardOpenOption.CREATE,
      java.nio.file.StandardOpenOption.APPEND,
    )
    agents?.mcp(caller, name, sanitized, response)
  }
}

private fun jsonType(type: String) = buildJsonObject { put("type", type) }

private fun JsonObject.requireString(name: String): String =
  (get(name) as? JsonPrimitive)?.takeIf(JsonPrimitive::isString)?.content
    ?: error("$name must be a string")

private fun JsonElement.successResult(): JsonObject = buildJsonObject {
  put(
    "content",
    JsonArray(listOf(buildJsonObject {
      put("type", "text")
      put("text", wireJson.encodeToString(this@successResult))
    })),
  )
  put("structuredContent", this@successResult)
  put("isError", false)
}

private fun errorResult(message: String): JsonObject = buildJsonObject {
  put("content", JsonArray(listOf(buildJsonObject { put("type", "text"); put("text", message) })))
  put("isError", true)
}

private fun JsonObject.toToolResult(): CallToolResult = CallToolResult(
  content = (get("content") as? JsonArray).orEmpty().mapNotNull { item ->
    item.jsonObject.text("text").takeIf(String::isNotEmpty)?.let(::TextContent)
  },
  isError = flag("isError").takeIf { it == true },
  structuredContent = get("structuredContent") as? JsonObject,
)
