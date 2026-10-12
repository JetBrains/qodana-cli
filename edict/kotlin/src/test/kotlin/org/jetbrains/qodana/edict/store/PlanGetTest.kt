package org.jetbrains.qodana.edict.store

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.support.EdictNextTestTools
import org.jetbrains.qodana.edict.support.batch
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.test.assertEquals

class PlanGetTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `a coordinator reads one task instead of the whole plan`() {
    EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
      val (_, batch) = store.batch()
      val tools = EdictNextTestTools(store, layout = EdictLayout(directory, directory.resolve(".edict")))
      fun planGet(vararg arguments: Pair<String, String>): JsonObject =
        tools.call("edict_plan_get", buildJsonObject { arguments.forEach { (key, value) -> put(key, value) } })

      val plan = checkNotNull(planGet()["structuredContent"]).jsonObject.getValue("plan").jsonObject
      assertEquals(listOf(batch.taskId), plan.getValue("tasks").jsonArray.map { it.jsonObject.text("id") })

      val task = checkNotNull(planGet("taskId" to batch.taskId)["structuredContent"]).jsonObject.getValue("task").jsonObject
      assertEquals(batch.taskId, task.text("id"))
      assertEquals("running", task.text("status"))

      assertEquals(true, planGet("taskId" to "missing").flag("isError"))
    }
  }
}
