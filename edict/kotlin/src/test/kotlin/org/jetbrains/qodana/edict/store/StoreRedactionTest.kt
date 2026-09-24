package org.jetbrains.qodana.edict.store

import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.mcp.McpServer
import org.jetbrains.qodana.edict.support.batch
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Files
import java.nio.file.Path
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse

class StoreRedactionTest {
    @TempDir
    lateinit var directory: Path

    @Test
    fun `redaction finds embedded adjacent and revoked capabilities without hiding unrelated digests`() {
        Store(directory.resolve("state")).use { store ->
            val (manager, batch) = store.batch()
            for (offset in 0..64) {
                val prefix = "a".repeat(offset)
                assertEquals("${prefix}[REDACTED]ff", store.redact("$prefix${batch.token}ff"))
            }
            assertEquals("[REDACTED][REDACTED]", store.redact(manager.token + batch.token))
            val digest = "f".repeat(128)
            assertEquals(digest, store.redact(digest))
            assertEquals("no credentials", store.redact("no credentials"))
            store.finishTask(batch.token, "completed", "Done")
            assertEquals("a[REDACTED]b", store.redact("a${batch.token}b"))
        }
    }

    @Test
    fun `embedded capabilities cannot be saved in task titles or results`() {
        Store(directory.resolve("state")).use { store ->
            val (manager, batch) = store.batch()
            val before = store.plan()
            assertFailsWith<IllegalArgumentException> {
                store.addTask(batch.token, "edict-signal-analysis", "Receipt: a${manager.token}")
            }
            assertFailsWith<IllegalArgumentException> {
                store.finishTask(batch.token, "completed", "Receipt: a${batch.token}")
            }
            assertEquals(before, store.plan())
            val persisted = store.read("plans/${before!!.id}.json").content
            listOf(manager.token, batch.token).forEach { assertFalse(persisted.contains(it)) }
        }
    }

    @Test
    fun `rejected arguments do not leak embedded credentials into MCP or agent logs`() {
        Store(directory.resolve("state")).use { store ->
            val (manager, batch) = store.batch()
            val logs = directory.resolve("logs")
            val response = McpServer(store, logs = logs).call("edict_task_add", buildJsonObject {
                put("token", batch.token)
                put("skill", "edict-signal-analysis")
                put("title", "Receipt: a${manager.token}f${batch.token}")
            })
            assertEquals(true, response.flag("isError"))
            assertEquals(1, store.plan()!!.tasks.size)
            Files.list(logs).use { paths ->
                paths.forEach { file ->
                    val content = Files.readString(file).replace("\n    ", "")
                    listOf(manager.token, batch.token).forEach { assertFalse(content.contains(it), file.toString()) }
                }
            }
            assertContains(Files.readString(logs.resolve("edict-mcp-system.log")), "a[REDACTED]f[REDACTED]")
        }
    }
}
