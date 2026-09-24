package org.jetbrains.qodana.edict.mcp

import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.nio.file.Path
import kotlin.test.*
import org.jetbrains.qodana.edict.store.Store
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

class McpHttpTest {
    @TempDir lateinit var directory: Path

    private val createPlan = """{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"edict_plan_create","arguments":{"request":"Extract","steps":[{"skill":"edict-batch-signal-analysis","title":"Commit"}]}}}"""
    private val ping = """{"jsonrpc":"2.0","id":1,"method":"ping"}"""

    private fun HttpClient.post(url: String, body: String = createPlan, origins: List<String> = emptyList(), contentType: String? = "application/json"): HttpResponse<String> {
        val request = HttpRequest.newBuilder(URI(url))
        origins.forEach { request.header("Origin", it) }
        contentType?.let { request.header("Content-Type", it) }
        return send(request.POST(HttpRequest.BodyPublishers.ofString(body)).build(), HttpResponse.BodyHandlers.ofString())
    }

    @Test fun `untrusted and malformed origins cannot claim manager authority`() {
        Store(directory.resolve("state")).use { store ->
            McpServer(store).serveHttp().use { transport ->
                HttpClient.newHttpClient().use { client ->
                    val origin = transport.url.removeSuffix("/mcp")
                    listOf(
                        listOf("https://untrusted.example"), listOf("null"), listOf("not-an-origin"),
                        listOf("http://127.0.0.1:1"), listOf("http://localhost:1"),
                        listOf("$origin/path"), listOf("$origin?query"),
                        listOf("$origin https://untrusted.example"), listOf(origin, origin),
                    ).forEach { origins ->
                        assertEquals(403, client.post(transport.url, origins = origins).statusCode(), origins.toString())
                        assertNull(store.plan(), "Rejected request must not create a plan")
                    }
                    assertEquals(403, client.post(transport.url, origins = listOf("https://untrusted.example"), contentType = "text/plain").statusCode())
                    assertEquals(200, client.post(transport.url).statusCode())
                    assertEquals("Extract", assertNotNull(store.plan()).request)
                }
            }
        }
    }

    @Test fun `non JSON posts cannot mutate state`() {
        Store(directory.resolve("state")).use { store ->
            McpServer(store).serveHttp().use { transport ->
                HttpClient.newHttpClient().use { client ->
                    listOf(null, "text/plain", "application/x-www-form-urlencoded", "multipart/form-data").forEach { type ->
                        assertEquals(415, client.post(transport.url, contentType = type).statusCode())
                        assertNull(store.plan())
                    }
                    assertEquals(200, client.post(transport.url, contentType = "application/json; charset=utf-8").statusCode())
                    assertNotNull(store.plan())
                }
            }
        }
    }

    @Test fun `native and same origin clients retain HTTP access`() {
        Store(directory.resolve("state")).use { store ->
            McpServer(store).serveHttp().use { transport ->
                HttpClient.newHttpClient().use { client ->
                    val origin = transport.url.removeSuffix("/mcp")
                    listOf(emptyList(), listOf(origin), listOf(origin.replace("127.0.0.1", "localhost"))).forEach { origins ->
                        val response = client.post(transport.url, body = ping, origins = origins)
                        assertEquals(200, response.statusCode())
                        assertContains(response.body(), "\"result\":{}")
                    }
                }
            }
        }
    }
}
