package org.jetbrains.qodana.edict.integration.support.inspection

import com.sun.net.httpserver.HttpServer
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.common.wireJson
import java.net.InetSocketAddress
import java.nio.file.Files
import java.nio.file.Path
import java.util.concurrent.Executors
import kotlin.test.assertTrue

/** Minimal IntelliJ MCP lifecycle used when a test does not execute inspection tools. */
internal class InspectionLifecycleFixture(directory: Path) : AutoCloseable {
    private val executor = Executors.newCachedThreadPool()
    private val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0).apply {
        this.executor = this@InspectionLifecycleFixture.executor
        createContext("/mcp") { exchange ->
            exchange.use {
                val request = wireJson.parseToJsonElement(exchange.requestBody.readAllBytes().decodeToString()).jsonObject
                if (request["id"] == null) {
                    exchange.sendResponseHeaders(202, -1)
                    return@createContext
                }
                val response = buildJsonObject {
                    put("jsonrpc", "2.0")
                    put("id", request.getValue("id"))
                    put("result", buildJsonObject {
                        put("protocolVersion", "2025-03-26")
                        put("capabilities", buildJsonObject {})
                        put("serverInfo", buildJsonObject {
                            put("name", "inspection-test")
                            put("version", "1")
                        })
                    })
                }.toString().encodeToByteArray()
                exchange.responseHeaders.set("Content-Type", "application/json")
                exchange.sendResponseHeaders(200, response.size.toLong())
                exchange.responseBody.write(response)
            }
        }
        start()
    }

    val qodanaExecutable: Path = directory.resolve("qodana-lifecycle").also { executable ->
        Files.writeString(executable, """
            #!/bin/sh
            if [ "${'$'}3" = "start" ]; then
              printf '%s\n' '{"status":"ready","url":"http://127.0.0.1:${server.address.port}/mcp"}'
            fi
        """.trimIndent())
        assertTrue(executable.toFile().setExecutable(true), "Cannot make fake Qodana executable")
    }

    override fun close() {
        server.stop(0)
        executor.shutdownNow()
    }
}
