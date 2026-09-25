// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import com.sun.net.httpserver.HttpServer
import kotlinx.serialization.json.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Timeout
import org.junit.jupiter.api.io.TempDir
import org.jetbrains.qodana.edict.signals.SignalValidation
import org.jetbrains.qodana.edict.model.SignalLabel
import org.jetbrains.qodana.edict.model.Step
import org.jetbrains.qodana.edict.store.Store
import java.net.InetSocketAddress
import java.net.URI
import java.nio.file.Path
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import kotlin.io.path.*
import kotlin.test.*

class GenerationTest {
    @TempDir lateinit var root: Path

    @Test fun `server and scan override image configuration without duplicate JVM properties`() {
        val server = qodanaProcess(root, root.resolve("mcp-results"), root.resolve("mcp-cache"), root.resolve("mcp.log"))
        val scan = qodanaProcess(root, root.resolve("scan-results"), root.resolve("scan-cache"), root.resolve("scan.log"))
        assertEquals(root.resolve("mcp-cache/config").toString(), server.environment()["QODANA_CONF"])
        assertEquals(root.resolve("scan-cache/config").toString(), scan.environment()["QODANA_CONF"])
        assertFalse((server.command() + scan.command()).any { it.contains("idea.config.path") })
    }

    @Test fun `generation holds optional fixtures out and preserves original required labels`() {
        commandOutput("git", "init", "--quiet", root.toString())
        root.resolve("X.java").writeText("class X {\n}\n")
        commandOutput("git", "-C", root.toString(), "add", "X.java")
        commandOutput("git", "-C", root.toString(), "-c", "user.name=Test", "-c", "user.email=test@example.invalid",
            "commit", "--quiet", "--allow-empty", "-m", "Fixture")
        val required = obj("path" to text("X.java"), "expectedProblemRanges" to JsonArray(listOf(obj("start" to JsonPrimitive(1), "end" to JsonPrimitive(3)))))
        val specification = obj("ruleId" to text("Rule"), "description" to text("Original"), "language" to text("Java"),
            "positiveExamples" to JsonArray(listOf(required)), "negativeExamples" to JsonArray(listOf(required)),
            "optionalPositiveExamples" to JsonArray(listOf(obj("path" to text("HeldOut.java")))))
        writeJson(root.resolve("benchmark/Rule/specification.json"), specification)
        writeJson(root.resolve("benchmark/Other/specification.json"), JsonObject(specification + ("ruleId" to text("Other"))))
        val output = root.resolve("output").createDirectories()
        assertEquals(mapOf("rule" to "Rule"), prepareGeneration(root, output, 0, setOf("Rule")))
        val cluster = readObject(output.resolve("state/clusters/rule/description.json"))
        val evidence = cluster.getValue("benchmarkSpecification").jsonObject
        assertEquals("Pending", cluster.string("status"))
        assertEquals(evidence["positiveExamples"], evidence["negativeExamples"])
        assertFalse(evidence.containsKey("optionalPositiveExamples"))
        assertEquals(specification, readObject(output.resolve("inputs.json")).getValue("specifications").jsonArray.single())
        assertEquals(specification, readObject(root.resolve("benchmark/Rule/specification.json")))
        Store(output.resolve("state")).use { store ->
            val inbox = store.list("inbox")
            assertEquals(2, inbox.size)
            val signals = inbox.map { SignalValidation.validate(it, store.read(it).content) }
            assertEquals(setOf(SignalLabel.POSITIVE, SignalLabel.NEGATIVE), signals.map { it.label }.toSet())
            assertEquals(1, signals.map { it.fileRevision }.distinct().size)
            assertEquals(3, signals.first().fileRevision.expectedRanges.single().end, "Preserve trailing context beyond EOF")
            assertTrue(signals.all { it.source.type == "SubmittedFeedback" && it.source.diffPositiveToNegative.isEmpty() })
            assertEquals(commandOutput("git", "-C", root.toString(), "rev-parse", "HEAD"), signals.first().fileRevision.revision)
            assertTrue(signals.all { it.provenance.workItemId.startsWith("benchmark/Rule/specification.json#/") })
            assertTrue(signals.none { it.toString().contains("HeldOut") })
            // Exercise the same validated MCP-store writes used by managed distribution.
            val manager = store.createPlan("Process benchmark inbox", listOf(Step("edict-distribution", "Distribute")))
            val worker = store.delegate(manager.token, manager.plan.tasks.single().id,
                listOf("inbox.delete", "cluster.signal.write"), listOf("inbox", "clusters/rule"),
                "\$edict-distribution\nProcess benchmark feedback.")
            store.readTask(worker.token)
            store.startTask(worker.token, "distribution-test", worker.skill)
            inbox.forEach { path ->
                val file = store.read(path)
                val target = "clusters/rule/signals/${path.substringAfterLast('/')}"
                store.write(worker.token, target, file.content, "")
                assertEquals(file.hash, store.read(target).hash)
                store.delete(worker.token, path, file.hash)
            }
            store.finishTask(worker.token, "completed", "All feedback distributed")
            assertTrue(store.list("inbox").isEmpty())
            assertEquals(2, store.list("clusters/rule/signals").size)
        }
        val prompt = output.resolve("prompt.txt").readText()
        assertContains(prompt, "Process the existing inbox and generate inspection rules")
        assertContains(prompt, "Delegate edict-run")
        assertFailsWith<IllegalArgumentException> { prepareGeneration(root, output, 0, setOf("Missing")) }
    }

    @Test fun `scan source copy excludes state gold and previous candidates`() {
        val source = root.resolve("source").createDirectories()
        for (path in listOf("src/X.java", "benchmark/gold.sarif.json", "inspections/Old.inspection.kts", ".edict/state.json", ".git/config", "qodana.yaml")) {
            source.resolve(path).apply { parent.createDirectories(); writeText(path) }
        }
        val target = root.resolve("target")
        copySource(source, target)
        assertEquals("src/X.java", target.resolve("src/X.java").readText())
        assertEquals(listOf("src"), target.listDirectoryEntries().map { it.fileName.toString() })
    }

    @Test @Timeout(15) fun `MCP client attaches and closes persistent GET stream and parses SSE replies`() {
        val executor = Executors.newCachedThreadPool()
        val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0)
        val attached = CountDownLatch(1)
        val release = CountDownLatch(1)
        server.executor = executor
        server.createContext("/mcp") { exchange ->
            exchange.use {
                if (exchange.requestMethod == "GET") {
                    assertEquals("test-session", exchange.requestHeaders.getFirst("Mcp-Session-Id"))
                    exchange.responseHeaders.set("Content-Type", "text/event-stream")
                    exchange.sendResponseHeaders(200, 0)
                    exchange.responseBody.write(": attached\n\n".toByteArray())
                    exchange.responseBody.flush()
                    attached.countDown()
                    release.await(10, TimeUnit.SECONDS)
                } else {
                    val request = json.parseToJsonElement(exchange.requestBody.readAllBytes().decodeToString()).jsonObject
                    if ("id" !in request) { exchange.sendResponseHeaders(202, -1); return@createContext }
                    val initialize = request.string("method") == "initialize"
                    if (!initialize && !attached.await(2, TimeUnit.SECONDS)) { exchange.sendResponseHeaders(404, -1); return@createContext }
                    val result = if (initialize) obj("protocolVersion" to text("2025-03-26")) else obj("tools" to JsonArray(emptyList()))
                    val message = obj("jsonrpc" to text("2.0"), "id" to request.getValue("id"), "result" to result)
                    val body = if (initialize) message.toString() else "event: message\ndata: $message\n\n"
                    exchange.responseHeaders.set("Content-Type", if (initialize) "application/json" else "text/event-stream")
                    exchange.responseHeaders.set("Mcp-Session-Id", "test-session")
                    exchange.sendResponseHeaders(200, body.toByteArray().size.toLong())
                    exchange.responseBody.write(body.toByteArray())
                }
            }
        }
        server.start()
        try {
            val client = InspectionClient(URI("http://127.0.0.1:${server.address.port}/mcp"))
            client.use { assertEquals(JsonArray(emptyList()), it.request("tools/list")["tools"]) }
            assertFailsWith<InfrastructureFailure> { client.request("tools/list") }
        } finally {
            release.countDown()
            server.stop(0)
            executor.shutdownNow()
        }
    }
}
