// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.support

import kotlinx.serialization.json.JsonObject
import org.jetbrains.qodana.edict.core.InspectionKtsBatchRunResult
import org.jetbrains.qodana.edict.core.InspectionKtsClient
import org.jetbrains.qodana.edict.core.InspectionKtsCompileResult
import org.jetbrains.qodana.edict.core.InspectionKtsExampleRequest
import org.jetbrains.qodana.edict.core.InspectionKtsProjectRunResult
import org.jetbrains.qodana.edict.core.IntellijMcpServerLifecycle
import java.net.URI
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.atomic.AtomicInteger

/**
 * An IDE client that answers each operation with what a test gives it; an operation without an answer fails as
 * unexpected. It records the tools called and how often it was closed.
 */
internal class FakeInspectionKtsClient(
  private val compilation: (code: String) -> InspectionKtsCompileResult = { unexpected("compilation") },
  private val examples: (code: String, examples: List<InspectionKtsExampleRequest>) -> InspectionKtsBatchRunResult =
    { _, _ -> unexpected("example run") },
  private val projectAnalysis: (code: String) -> InspectionKtsProjectRunResult = { unexpected("project analysis") },
  private val tools: (name: String, arguments: JsonObject) -> JsonObject = { name, _ -> unexpected("call of $name") },
  /** Thrown by every [close], after it is counted. */
  private val closeFailure: RuntimeException? = null,
) : InspectionKtsClient {
  /** The name and arguments of each tool called. */
  val toolCalls = CopyOnWriteArrayList<Pair<String, JsonObject>>()

  private val closeCount = AtomicInteger()
  val closes: Int get() = closeCount.get()

  override suspend fun compile(code: String): InspectionKtsCompileResult = compilation(code)

  override suspend fun runExamples(code: String, examples: List<InspectionKtsExampleRequest>): InspectionKtsBatchRunResult =
    this.examples(code, examples)

  override suspend fun analyzeProject(code: String): InspectionKtsProjectRunResult = projectAnalysis(code)

  override suspend fun callTool(name: String, arguments: JsonObject): JsonObject {
    toolCalls += name to arguments
    return tools(name, arguments)
  }

  override fun close() {
    closeCount.incrementAndGet()
    closeFailure?.let { throw it }
  }
}

/** An IDE MCP server that is already running at [endpoint]: it is never really started or stopped, only counted. */
internal class FakeIntellijMcpServerLifecycle(private val endpoint: URI = URI("http://127.0.0.1:1/mcp")) : IntellijMcpServerLifecycle {
  private val startCount = AtomicInteger()
  private val stopCount = AtomicInteger()
  val starts: Int get() = startCount.get()
  val stops: Int get() = stopCount.get()

  override suspend fun start(): URI = endpoint.also { startCount.incrementAndGet() }

  override suspend fun stop() {
    stopCount.incrementAndGet()
  }
}

private fun unexpected(operation: String): Nothing = error("Unexpected IDE $operation")
