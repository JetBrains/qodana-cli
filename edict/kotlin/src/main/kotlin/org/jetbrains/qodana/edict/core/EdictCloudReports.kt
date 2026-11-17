package org.jetbrains.qodana.edict.core

import kotlinx.coroutines.CancellationException
import kotlinx.serialization.json.*
import org.jetbrains.qodana.cloudclient.*
import org.jetbrains.qodana.cloudclient.s3.QDCloudS3Client
import org.jetbrains.qodana.edict.common.GitRepository
import org.jetbrains.qodana.publisher.PublishResult
import org.jetbrains.qodana.publisher.Publisher
import org.jetbrains.qodana.publisher.PublisherParameters
import org.jetbrains.qodana.publisher.schemas.PublisherReportType
import java.net.URI
import java.net.http.HttpClient
import java.nio.file.Files
import java.nio.file.Path
import java.util.UUID
import kotlin.io.path.writeText

internal fun interface EdictReportUploader {
  suspend fun upload(project: Path, inspectionName: String, findings: EdictInspectionFindings): String
}

internal fun validReportUrl(value: String): Boolean {
  val uri = runCatching { URI(value) }.getOrNull() ?: return false
  return uri.scheme in setOf("http", "https") && !uri.host.isNullOrBlank() && uri.userInfo == null &&
    value.none { it.isWhitespace() || it in "<>\\()" }
}

/** Uploads only the final inspection's saved analysis; never reruns the project or exposes the project token to agents. */
internal object EdictCloudReports {
  fun fromEnvironment(environment: (String) -> String? = System::getenv): EdictReportUploader? {
    val token = environment("QODANA_TOKEN")?.takeIf { it.isNotBlank() } ?: return null
    val endpoint = environment("QODANA_ENDPOINT")?.takeIf { it.isNotBlank() } ?: "https://qodana.cloud"
    return EdictReportUploader { project, name, findings -> upload(token, endpoint, project, name, findings) }
  }

  private suspend fun upload(token: String, endpoint: String, project: Path, name: String, findings: EdictInspectionFindings): String {
    val directory = Files.createTempDirectory("edict-cloud-report-")
    try {
      val git = GitRepository(project)
      val origin = runCatching { git.git("remote", "get-url", "origin").trim() }.getOrNull()?.let { raw ->
        // Git's scp-style remote syntax is not a URI; SARIF requires one.
        val value = if (Regex("^[^/@]+@[^/:]+:.+").matches(raw)) "ssh://${raw.replaceFirst(':', '/')}" else raw
        runCatching { URI(value).takeIf { it.isAbsolute }?.toASCIIString() }.getOrNull()
      }
      val branch = runCatching { git.currentBranch() }.getOrNull()
      val sources = mutableMapOf<Pair<String, String>, String?>()
      val report = sarif(name, findings, origin, branch) { file ->
        val key = file.revision to file.path
        if (key !in sources) sources[key] = runCatching { git.fileAt(file.revision, file.path) }.getOrNull()
        sources[key]
      }
      directory.resolve("qodana.sarif.json").writeText(report.toString())
      val http = HttpClient.newBuilder().connectTimeout(java.time.Duration.ofSeconds(30)).build()
      val cloudHttp = QDCloudHttpClient(http)
      val api = QDCloudClient(cloudHttp, QDCloudEnvironment(endpoint, cloudHttp)).v1()
      check(api is QDCloudResponse.Success) { "Could not connect to Qodana Cloud" }
      val result = Publisher(api.value.projectApi(token), QDCloudS3Client(http)).publish(PublisherParameters(
        reportPath = directory, reportType = PublisherReportType.SARIF,
        commit = findings.projectRevision, branch = branch,
      ))
      check(result is PublishResult.Success) { "Could not upload the final inspection report to Qodana Cloud" }
      return result.uploadedReport.reportLink
    }
    catch (e: CancellationException) { throw e }
    catch (_: Exception) {
      // Publisher failures can contain authenticated URLs or server payloads. Keep credentials out of tool results.
      error("Qodana Cloud report upload failed; generation has not been finalized")
    }
    finally {
      Files.walk(directory).use { paths -> paths.sorted(Comparator.reverseOrder()).forEach(Files::deleteIfExists) }
    }
  }

  internal fun sarif(
    name: String, findings: EdictInspectionFindings, origin: String?, branch: String?,
    sourceAt: (EdictFileRevision) -> String? = { null },
  ): JsonObject = buildJsonObject {
    put("version", "2.1.0")
    put("\$schema", "https://json.schemastore.org/sarif-2.1.0.json")
    putJsonArray("runs") {
      addJsonObject {
        putJsonObject("tool") {
          putJsonObject("driver") {
            put("name", "Edict"); put("fullName", "Edict")
            putJsonArray("rules") {
              addJsonObject {
                put("id", findings.clusterId)
                putJsonObject("shortDescription") { put("text", name) }
                putJsonObject("fullDescription") { put("text", findings.inspectionDescription) }
                // Cloud processing requires a rule configuration even when every result specifies its level.
                putJsonObject("defaultConfiguration") { put("enabled", true); put("level", "warning") }
              }
            }
          }
        }
        putJsonObject("automationDetails") { put("guid", UUID.randomUUID().toString()); put("id", "edict/${findings.clusterId}") }
        if (origin != null) putJsonArray("versionControlProvenance") {
          addJsonObject {
            put("repositoryUri", origin); put("revisionId", findings.projectRevision)
            branch?.let { put("branch", it) }
          }
        }
        putJsonArray("results") {
          findings.findings.forEach { finding ->
            val regions = sourceRegions(finding, sourceAt(finding.fileRevision))
            val language = findings.language.name.lowercase()
            addJsonObject {
              put("ruleId", findings.clusterId); put("level", "warning")
              putJsonObject("message") { put("text", finding.message ?: name) }
              putJsonArray("locations") {
                addJsonObject {
                  putJsonObject("physicalLocation") {
                    putJsonObject("artifactLocation") {
                      put("uri", URI(null, null, finding.fileRevision.path, null).toASCIIString())
                      put("uriBaseId", "SRCROOT")
                    }
                    if (regions != null) {
                      put("region", regions.first.json(language))
                      put("contextRegion", regions.second.json(language))
                    } else {
                      finding.fileRevision.expectedRanges?.firstOrNull()?.let { range ->
                        putJsonObject("region") { put("startLine", range.start); put("endLine", range.end) }
                      }
                      finding.contextSnippet?.let { snippet ->
                        putJsonObject("contextRegion") {
                          (finding.contextStartLine ?: finding.fileRevision.expectedRanges?.firstOrNull()?.start)?.let { put("startLine", it) }
                          // The Cloud converter omits code entirely without a character length.
                          put("charLength", snippet.length)
                          put("sourceLanguage", language)
                          putJsonObject("snippet") { put("text", snippet) }
                        }
                      }
                    }
                  }
                }
              }
            }
          }
        }
      }
    }
  }

  private data class SourceRegion(val text: String, val line: Int, val column: Int, val offset: Int) {
    fun json(language: String) = buildJsonObject {
      put("startLine", line); put("endLine", line + text.count { it == '\n' })
      put("startColumn", column); put("charOffset", offset); put("charLength", text.length)
      put("sourceLanguage", language)
      putJsonObject("snippet") { put("text", this@SourceRegion.text) }
    }
  }

  /** Read the analyzed revision, not the working tree, which may have changed since analysis. */
  private fun sourceRegions(finding: EdictProjectFinding, source: String?): Pair<SourceRegion, SourceRegion>? {
    if (source == null) return null
    val range = finding.fileRevision.expectedRanges?.firstOrNull()
    val line = range?.start ?: finding.contextStartLine ?: return null
    val starts = listOf(0) + source.indices.filter { source[it] == '\n' }.map { it + 1 }
    if (line !in 1..starts.size) return null
    fun lineEnd(number: Int): Int {
      val start = starts[number - 1]
      var end = starts.getOrNull(number) ?: source.length
      if (end > start && source.getOrNull(end - 1) == '\n') end--
      if (end > start && source.getOrNull(end - 1) == '\r') end--
      return end
    }
    val lineStart = starts[line - 1]
    val element = finding.contextSnippet?.takeIf { it.isNotEmpty() }
    val elementStart = element?.let { source.indexOf(it, lineStart) }
      ?.takeIf { it >= lineStart && it < lineEnd(line) }
    val start = elementStart ?: lineStart
    val end = if (elementStart != null) start + element.length
      else lineEnd((range?.end ?: line).coerceIn(line, starts.size))
    val highlighted = SourceRegion(source.substring(start, end), line, start - lineStart + 1, start)
    val contextLine = (line - 3).coerceAtLeast(1)
    val contextEndLine = (line + highlighted.text.count { it == '\n' } + 3).coerceAtMost(starts.size)
    val contextStart = starts[contextLine - 1]
    val context = SourceRegion(source.substring(contextStart, lineEnd(contextEndLine)), contextLine, 1, contextStart)
    return highlighted to context
  }
}
