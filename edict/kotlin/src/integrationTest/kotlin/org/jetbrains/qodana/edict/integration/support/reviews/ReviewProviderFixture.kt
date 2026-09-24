// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration.support.reviews

import com.sun.net.httpserver.HttpExchange
import com.sun.net.httpserver.HttpServer
import java.net.InetSocketAddress
import java.net.URLDecoder
import java.util.Base64
import java.util.concurrent.ConcurrentLinkedQueue
import kotlin.test.*
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.git.GitRepository
import org.jetbrains.qodana.edict.integration.support.historyBefore
import org.jetbrains.qodana.edict.integration.support.historyCommit
import org.jetbrains.qodana.edict.integration.support.historyPath
import org.jetbrains.qodana.edict.reviews.ReviewClient
import org.jetbrains.qodana.edict.reviews.github
import org.jetbrains.qodana.edict.reviews.space
import org.jetbrains.qodana.edict.support.batch

/** The real provider clients consume deterministic review APIs; source snapshots come from the Distillery clone. */
internal class ReviewProviderFixture(private val provider: String, repository: GitRepository) : AutoCloseable {
    val title = "Replace Thread.sleep with explicit worker synchronization"
    val message = "Please replace Thread.sleep with an explicit worker completion signal. The final commit uses CountDownLatch.await instead."
    val source = listOf(historyBefore, historyCommit).associateWith { repository.fileAt(it, historyPath) }
    private val errors = ConcurrentLinkedQueue<Throwable>()
    private val requests = ConcurrentLinkedQueue<String>()
    private val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0)
    private val baseUrl = "http://127.0.0.1:${server.address.port}"
    val discussionUrl = when (provider) {
        "github" -> "https://github.test/owner/repo/pull/7#discussion_r10"
        "space" -> "$baseUrl/im/review/review-id?message=root&channel=feed"
        else -> error("Unknown review fixture provider")
    }
    val client = ReviewClient(baseUrl, baseUrl, "fixture-review-token", "fixture-review-token")

    init {
        server.createContext("/") { exchange ->
            exchange.use {
                try {
                    assertEquals("GET", exchange.requestMethod)
                    assertEquals("Bearer fixture-review-token", exchange.requestHeaders.getFirst("Authorization"))
                    requests += exchange.requestURI.path
                    val bytes = response(exchange).toByteArray(Charsets.UTF_8)
                    exchange.responseHeaders.set("Content-Type", "application/json")
                    exchange.sendResponseHeaders(200, bytes.size.toLong())
                    exchange.responseBody.write(bytes)
                } catch (error: Throwable) {
                    errors += error
                    exchange.sendResponseHeaders(500, -1)
                }
            }
        }
        server.start()
    }

    private fun response(exchange: HttpExchange): String {
        val path = exchange.requestURI.path
        val query = exchange.requestURI.rawQuery.orEmpty().split('&').filter(String::isNotEmpty).associate {
            URLDecoder.decode(it.substringBefore('='), Charsets.UTF_8) to URLDecoder.decode(it.substringAfter('='), Charsets.UTF_8)
        }
        fun quote(value: String) = JsonPrimitive(value).toString()
        fun encoded(content: String) = Base64.getEncoder().encodeToString(content.toByteArray(Charsets.UTF_8))
        return when (path) {
            "/repos/owner/repo/pulls/7" -> {
                assertEquals("github", provider)
                """{"number":7,"title":${quote(title)},"body":"Correct worker synchronization","html_url":"https://github.test/owner/repo/pull/7","merged_at":"2026-09-21T12:00:00Z","base":{"sha":"$historyBefore"},"head":{"sha":"$historyCommit"}}"""
            }
            "/repos/owner/repo/pulls/7/comments" ->
                """[{"id":10,"path":"$historyPath","original_commit_id":"$historyBefore","original_line":5,"body":${quote(message)},"created_at":"2026-09-20T12:00:00Z","user":{"login":"reviewer","type":"User"}}]"""
            "/repos/owner/repo/pulls/7/reviews" -> "[]"
            "/repos/owner/repo/contents/$historyPath" -> {
                val content = source.getValue(query.getValue("ref"))
                """{"type":"file","encoding":"base64","size":${content.toByteArray().size},"content":"${encoded(content)}"}"""
            }
            "/api/http/projects/key:owner/repositories/repo/files" -> {
                val ref = query.getValue("commit")
                assertTrue(ref in source)
                assertEquals(historyPath, query["path"])
                """[{"path":"$historyPath","type":"FILE","blob":"$ref"}]"""
            }
            "/api/http/projects/key:owner/repositories/repo/content" -> {
                val content = source.getValue(query.getValue("blobId"))
                """{"totalSize":${content.toByteArray().size},"partBase64":"${encoded(content)}"}"""
            }
            "/api/http/projects/key:owner/code-reviews/number:7" -> {
                assertEquals("space", provider)
                """{"id":"review-id","number":7,"state":"Closed","timestamp":1790000000000,"title":${quote(title)},"description":"Correct worker synchronization","feedChannelId":"feed","branchPair":{"repository":"repo","isMerged":true,"sourceBranchRef":"$historyCommit","targetBranchInfo":{"ref":"$historyBefore"}}}"""
            }
            "/api/http/chats/messages/sync-batch" ->
                """{"data":[{"chatMessage":{"id":"root","projectedItem":{"author":{"name":"Reviewer","details":{"user":{"id":"person"}}}},"details":{"className":"CodeDiscussionAddedFeedEvent","codeDiscussion":{"id":"discussion","channel":{"id":"thread"},"anchor":{"filename":"$historyPath","line":5,"revision":"$historyBefore"}}}}}],"etag":"done","hasMore":false}"""
            "/api/http/chats/messages" ->
                """{"messages":[{"id":"comment","text":${quote(message)},"author":{"name":"Reviewer"},"created":{"iso":"2026-09-20T12:00:00Z"}}],"orgLimitReached":false,"nextStartFromDate":null}"""
            else -> error("Unexpected review API request: $path")
        }
    }

    fun verifyRequests() {
        errors.firstOrNull()?.let { throw it }
        val required = if (provider == "github") listOf(
            "/repos/owner/repo/pulls/7", "/repos/owner/repo/pulls/7/comments", "/repos/owner/repo/pulls/7/reviews",
        ) else listOf(
            "/api/http/projects/key:owner/code-reviews/number:7", "/api/http/chats/messages/sync-batch", "/api/http/chats/messages",
        )
        required.forEach { assertTrue(it in requests, "Real $provider client must request $it") }
    }

    override fun close() {
        server.stop(0)
        errors.firstOrNull()?.let { throw it }
    }
}
