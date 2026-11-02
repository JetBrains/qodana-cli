package org.jetbrains.qodana.edict.edictnext

import com.sun.net.httpserver.HttpServer
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.runProcess
import org.junit.jupiter.api.io.TempDir
import java.net.InetSocketAddress
import java.nio.file.Path
import java.util.concurrent.CopyOnWriteArrayList
import java.util.zip.GZIPInputStream
import kotlin.io.path.createDirectories
import kotlin.io.path.writeText
import kotlin.test.*

class EdictNextCloudReportsTest {
  @TempDir lateinit var directory: Path

  @Test
  fun `missing or blank token disables cloud uploads`() {
    assertNull(EdictNextCloudReports.fromEnvironment { null })
    assertNull(EdictNextCloudReports.fromEnvironment { " " })
  }

  @Test
  fun `publisher uploads saved findings and returns finished report URL without sending token to storage`() = runBlocking {
    val requests = CopyOnWriteArrayList<Pair<String, String?>>()
    val uploaded = CopyOnWriteArrayList<JsonObject>()
    val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0)
    val endpoint = "http://127.0.0.1:${server.address.port}"
    server.createContext("/") { exchange ->
      val path = exchange.requestURI.path
      requests += path to exchange.requestHeaders.getFirst("Authorization")
      val response = when (path) {
        "/api/versions" -> """{"api":{"versions":[{"version":"1.0","url":"$endpoint/api/"}]}}"""
        "/api/reports/" -> """{"reportId":"report-id","fileLinks":{"qodana.sarif.json":"$endpoint/storage"},"langsRequired":false}"""
        "/storage" -> {
          val bytes = exchange.requestBody.readAllBytes()
          val json = GZIPInputStream(bytes.inputStream()).use { it.readAllBytes().decodeToString() }
          uploaded += Json.parseToJsonElement(json).jsonObject
          ""
        }
        "/api/reports/report-id/finish/" -> """{"token":"report-token","url":"https://qodana.cloud/reports/report-id"}"""
        else -> error("Unexpected cloud request: $path")
      }
      val bytes = response.toByteArray()
      exchange.sendResponseHeaders(200, if (bytes.isEmpty()) -1 else bytes.size.toLong())
      exchange.responseBody.use { it.write(bytes) }
      exchange.close()
    }
    server.start()
    try {
      runProcess(directory, listOf("git", "init", "--quiet"))
      runProcess(directory, listOf("git", "remote", "add", "origin", "git@github.com:org/repo.git"))
      val source = "package sample\r\n\r\n// 🧭\r\n\r\n@Suppress(\"unused\")\r\n// test\r\nclass SampleTest {\r\n  fun test() = Unit\r\n}\r\n"
      val file = directory.resolve("src/Sample File.kt")
      file.parent.createDirectories()
      file.writeText(source)
      runProcess(directory, listOf("git", "-c", "core.autocrlf=false", "add", "."))
      runProcess(directory, listOf("git", "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--quiet", "-m", "Source"))
      val revision = runProcess(directory, listOf("git", "rev-parse", "HEAD")).trim()
      file.writeText("working tree changed after analysis")
      val uploader = checkNotNull(EdictNextCloudReports.fromEnvironment {
        when (it) { "QODANA_TOKEN" -> "project-token"; "QODANA_ENDPOINT" -> endpoint; else -> null }
      })
      val url = uploader.upload(directory, "Sample inspection", findings(revision))
      assertEquals("https://qodana.cloud/reports/report-id", url)
      val run = uploaded.single()["runs"]!!.jsonArray.single().jsonObject
      // The live Cloud processor rejects the report when this optional SARIF object is absent.
      val rule = run["tool"]!!.jsonObject["driver"]!!.jsonObject["rules"]!!.jsonArray.single().jsonObject
      val configuration = rule["defaultConfiguration"]!!.jsonObject
      assertEquals(true, configuration["enabled"]!!.jsonPrimitive.boolean)
      assertEquals("warning", configuration["level"]!!.jsonPrimitive.content)
      assertEquals("ssh://git@github.com/org/repo.git", run["versionControlProvenance"]!!.jsonArray.single().jsonObject["repositoryUri"]!!.jsonPrimitive.content)
      val result = run["results"]!!.jsonArray.single().jsonObject
      assertEquals("sample-rule", result["ruleId"]!!.jsonPrimitive.content)
      val physical = result["locations"]!!.jsonArray.single().jsonObject["physicalLocation"]!!.jsonObject
      assertEquals("src/Sample%20File.kt", physical["artifactLocation"]!!.jsonObject["uri"]!!.jsonPrimitive.content)
      assertEquals(7, physical["region"]!!.jsonObject["startLine"]!!.jsonPrimitive.int)
      val region = physical["region"]!!.jsonObject
      assertEquals(7, region["startColumn"]!!.jsonPrimitive.int)
      assertEquals(source.indexOf("SampleTest"), region["charOffset"]!!.jsonPrimitive.int)
      assertEquals("SampleTest".length, region["charLength"]!!.jsonPrimitive.int)
      val context = physical["contextRegion"]!!.jsonObject
      val snippet = context["snippet"]!!.jsonObject["text"]!!.jsonPrimitive.content
      assertTrue(snippet.contains("class SampleTest {\r\n  fun test() = Unit"))
      assertFalse(snippet.contains("working tree changed"))
      assertEquals(snippet.length, context["charLength"]!!.jsonPrimitive.int)
      assertEquals("kotlin", context["sourceLanguage"]!!.jsonPrimitive.content)
      assertEquals(source.indexOf("SampleTest") - context["charOffset"]!!.jsonPrimitive.int,
        snippet.indexOf("SampleTest"))
      assertTrue(requests.filter { it.first.startsWith("/api/reports/") }.all { it.second == "Bearer project-token" })
      assertNull(requests.single { it.first == "/storage" }.second)
    } finally { server.stop(0) }
  }

  @Test
  fun `saved snippet remains renderable when historical source is unavailable`() {
    val run = EdictNextCloudReports.sarif("Sample", findings(), null, null)["runs"]!!.jsonArray.single().jsonObject
    val context = run["results"]!!.jsonArray.single().jsonObject["locations"]!!.jsonArray.single().jsonObject[
      "physicalLocation"]!!.jsonObject["contextRegion"]!!.jsonObject
    assertEquals(7, context["startLine"]!!.jsonPrimitive.int)
    assertEquals("SampleTest", context["snippet"]!!.jsonObject["text"]!!.jsonPrimitive.content)
    assertEquals("SampleTest".length, context["charLength"]!!.jsonPrimitive.int)
  }

  @Test
  fun `line range provides highlighted source when no element text was saved`() {
    val source = "class SampleTest {\n  fun test() = Unit\n}\n"
    val findings = findings().let { report -> report.copy(findings = listOf(report.findings.single().copy(
      contextSnippet = null,
      fileRevision = report.findings.single().fileRevision.copy(expectedRanges = listOf(EdictNextLineRange(1, 3))),
    ))) }
    val run = EdictNextCloudReports.sarif("Sample", findings, null, null) { source }["runs"]!!.jsonArray.single().jsonObject
    val physical = run["results"]!!.jsonArray.single().jsonObject["locations"]!!.jsonArray.single().jsonObject[
      "physicalLocation"]!!.jsonObject
    val region = physical["region"]!!.jsonObject
    assertEquals(source.trimEnd(), region["snippet"]!!.jsonObject["text"]!!.jsonPrimitive.content)
    assertEquals(3, region["endLine"]!!.jsonPrimitive.int)
    assertEquals(source.length - 1, region["charLength"]!!.jsonPrimitive.int)
    assertEquals(source, physical["contextRegion"]!!.jsonObject["snippet"]!!.jsonObject["text"]!!.jsonPrimitive.content)
  }

  private fun findings(revision: String = "b".repeat(40)) = EdictNextInspectionFindings(
    clusterId = "sample-rule", candidateDigest = "a".repeat(64), projectRevision = revision,
    inspectionDescription = "Finds samples", language = EdictNextLanguage.Kotlin,
    findings = listOf(EdictNextProjectFinding(
      EdictNextFileRevision("src/Sample File.kt", revision, listOf(EdictNextLineRange(7, 7))),
      message = "Make this internal", contextSnippet = "SampleTest", contextStartLine = 7,
    )),
  )
}
