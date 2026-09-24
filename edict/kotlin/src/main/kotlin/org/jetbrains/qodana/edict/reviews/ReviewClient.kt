// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.reviews

import java.net.URI
import java.net.URLEncoder
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.nio.file.Files
import java.nio.file.NoSuchFileException
import java.time.Duration
import java.util.*
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.number
import org.jetbrains.qodana.edict.common.runProcess
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.signals.validRevision
import org.jetbrains.qodana.edict.signals.validSourcePath

internal fun urlPart(value: String): String = URLEncoder.encode(value, Charsets.UTF_8).replace("+", "%20")
internal fun isBot(name: String, kind: String): Boolean = kind.equals("bot", true) || name.lowercase().let {
    it.endsWith("[bot]") || it in listOf(
        "dependabot",
        "github-actions",
        "patronus",
        "space-automation",
        "jetbrains-bot"
    )
}

/** Read-only provider access. Credentials and API origins belong to the host, never to tool input. */
class ReviewClient(
    internal val githubUrl: String = System.getenv("EDICT_GITHUB_API_URL") ?: "https://api.github.com",
    internal val spaceUrl: String = System.getenv("EDICT_SPACE_URL") ?: "https://jetbrains.team",
    private val githubToken: String = System.getenv("GITHUB_TOKEN") ?: System.getenv("GH_TOKEN").orEmpty(),
    private val spaceToken: String = System.getenv("SPACE_TOKEN").orEmpty(),
) : ReviewProvider {
    private val http =
        HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(30)).followRedirects(HttpClient.Redirect.NEVER)
            .build()

    internal data class Response(val body: JsonElement, val hasNext: Boolean)

    internal fun get(provider: String, endpoint: String, query: Map<String, String> = emptyMap()): Response {
        val base = if (provider == "space") spaceUrl.trimEnd('/') + "/api/http" else githubUrl.trimEnd('/')
        val token = if (provider == "space") spaceToken else githubToken
        require(provider != "space" || token.isNotBlank()) { "Space access requires SPACE_TOKEN in server environment" }
        val uri = URI(base)
        require(
            uri.scheme in listOf(
                "https",
                "http"
            ) && uri.host != null && uri.userInfo == null && uri.query == null && uri.fragment == null
        ) { "Invalid provider API URL" }
        val suffix = query.entries.joinToString("&") { "${urlPart(it.key)}=${urlPart(it.value)}" }
        val builder = HttpRequest.newBuilder(URI("$base$endpoint?$suffix")).timeout(Duration.ofSeconds(30))
            .header("Accept", "application/json").header("User-Agent", "edict-mcp").GET()
        if (token.isNotEmpty()) builder.header("Authorization", "Bearer $token")
        val response = try {
            http.send(builder.build(), HttpResponse.BodyHandlers.ofInputStream())
        } catch (e: InterruptedException) {
            Thread.currentThread().interrupt(); throw e
        } catch (_: Exception) {
            error("$provider API request failed (connection or timeout)")
        }
        val body = response.body().use { input ->
            if (response.statusCode() == 404) throw NoSuchFileException("$provider source")
            check(response.statusCode() == 200) { "$provider API returned HTTP ${response.statusCode()}" }
            val bytes = input.readNBytes(16 * 1024 * 1024 + 1)
            require(bytes.size <= 16 * 1024 * 1024) { "Provider response exceeds 16 MiB; refusing incomplete evidence" }
            bytes.toString(Charsets.UTF_8)
        }
        val parsed = try {
            wireJson.parseToJsonElement(body)
        } catch (_: Exception) {
            error("Invalid $provider API JSON response")
        }
        return Response(parsed, response.headers().firstValue("Link").orElse("").contains("rel=\"next\""))
    }

    override fun fetch(selection: ReviewSelection): List<PullRequest> {
        selection.validate()
        val prs = if (selection.provider == "github") github(selection) else space(selection)
        prs.forEach { pr ->
            require(validRevision(pr.baseRevision) && validRevision(pr.headRevision)) { "PR ${pr.number} lacks exact base/head revisions" }
            pr.threads.forEach { t -> require(validSourcePath(t.filePath) && validRevision(t.originalCommitSha) && t.anchorLine > 0 && t.anchorEndLine >= t.anchorLine) { "Invalid discussion path, revision or anchors" } }
        }
        return if (selection.prNumbers.isEmpty()) prs.sortedWith(
            compareBy(
                PullRequest::closeTimestamp,
                PullRequest::number
            )
        ) else prs
    }

    internal fun endpoint(repo: ReviewRepository): String =
        if (repo.provider == "github") "/repos/${urlPart(repo.owner)}/${urlPart(repo.repo)}"
        else "/projects/key:${urlPart(repo.owner)}/repositories/${urlPart(repo.repo)}"

    override fun file(repository: ReviewRepository, revision: String, path: String): String {
        require(validRevision(revision) && validSourcePath(path)) { "Source reads require exact revision and repository-relative path" }
        val root = endpoint(repository)
        val encoded: String
        val size: Long
        if (repository.provider == "github") {
            val value = get(
                "github",
                "$root/contents/${path.split('/').joinToString("/", transform = ::urlPart)}",
                mapOf("ref" to revision)
            ).body.jsonObject
            require(value.text("type") == "file" && value.text("encoding") == "base64") { "GitHub did not return complete base64 file content" }
            encoded = value.text("content"); size = value.number("size")
        } else {
            val files = get("space", "$root/files", mapOf("commit" to revision, "path" to path)).body.jsonArray
            val file = files.map { it.jsonObject }
                .firstOrNull { it.text("path") == path && it.text("type") in listOf("FILE", "EXE_FILE") }
                ?: throw NoSuchFileException(path)
            val value = get(
                "space",
                "$root/content",
                mapOf("blobId" to file.text("blob"), "skip" to "0", "limit" to "16777216")
            ).body.jsonObject
            encoded = value.text("partBase64"); size = value.number("totalSize")
        }
        val bytes = Base64.getDecoder().decode(encoded.replace("\n", "").replace("\r", ""))
        require(bytes.size.toLong() == size && bytes.none { it == 0.toByte() }) { "Provider file content is invalid, binary or truncated" }
        return bytes.toString(Charsets.UTF_8)
    }

    override fun diff(
        repository: ReviewRepository,
        before: String,
        after: String,
        beforePath: String,
        afterPath: String
    ): String {
        fun readOrMissing(revision: String, path: String): String? = try {
            file(repository, revision, path)
        } catch (_: NoSuchFileException) {
            null
        }

        val old = readOrMissing(before, beforePath)
        val new = readOrMissing(after, afterPath)
        require(old != null || new != null) { "File absent at both revisions" }
        val scratch = Files.createTempDirectory("edict-review-diff-")
        try {
            // A temporary Git index gives Git's real unified diff without truncating provider compare patches.
            runProcess(scratch, listOf("git", "init", "-q"))
            Files.writeString(scratch.resolve("source"), old.orEmpty())
            runProcess(scratch, listOf("git", "add", "source"))
            Files.writeString(scratch.resolve("source"), new.orEmpty())
            val diff = runProcess(
                scratch,
                listOf(
                    "git",
                    "--no-pager",
                    "diff",
                    "--no-color",
                    "--no-ext-diff",
                    "--no-textconv",
                    "--unified=200",
                    "--",
                    "source"
                )
            )
            if (diff.isEmpty()) return ""
            val index = diff.indexOf("@@ ")
            require(index >= 0) { "Provider snapshot diff has no source hunks" }
            fun quoted(path: String): String = if (path.any { it in "\t\"\\" }) JsonPrimitive(path).toString() else path
            return "--- ${quoted(if (old == null) "/dev/null" else "a/$beforePath")}\n+++ ${quoted(if (new == null) "/dev/null" else "b/$afterPath")}\n${
                diff.substring(
                    index
                )
            }"
        } finally {
            Files.walk(scratch).use { it.sorted(Comparator.reverseOrder()).forEach(Files::delete) }
        }
    }
}
