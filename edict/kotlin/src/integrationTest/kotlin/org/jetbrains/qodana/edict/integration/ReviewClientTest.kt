package org.jetbrains.qodana.edict.integration

import com.sun.net.httpserver.HttpExchange
import com.sun.net.httpserver.HttpServer
import kotlinx.serialization.json.JsonPrimitive
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.integration.support.historyPath
import org.jetbrains.qodana.edict.model.Provenance
import org.jetbrains.qodana.edict.model.SignalSource
import org.jetbrains.qodana.edict.model.Step
import org.jetbrains.qodana.edict.reviews.*
import org.jetbrains.qodana.edict.signals.UnifiedDiff
import org.jetbrains.qodana.edict.store.Store
import org.jetbrains.qodana.edict.support.afterSource
import org.jetbrains.qodana.edict.support.beforeSource
import org.jetbrains.qodana.edict.support.fixturePath
import org.jetbrains.qodana.edict.support.launch
import org.junit.jupiter.api.Test
import java.net.InetSocketAddress
import java.net.URLDecoder
import java.util.*
import java.util.concurrent.ConcurrentLinkedQueue
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFails

class ReviewClientTest : IntegrationTest() {
    private val before = "a".repeat(40)
    private val after = "b".repeat(40)

    private fun fixture(handler: (HttpExchange) -> String, test: (ReviewClient) -> Unit) {
        val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0)
        val errors = ConcurrentLinkedQueue<Throwable>()
        server.createContext("/") { exchange ->
            exchange.use {
                try {
                    assertEquals("GET", exchange.requestMethod)
                    assertEquals("Bearer provider-secret", exchange.requestHeaders.getFirst("Authorization"))
                    val bytes = handler(exchange).toByteArray()
                    exchange.sendResponseHeaders(200, bytes.size.toLong())
                    exchange.responseBody.write(bytes)
                } catch (e: Throwable) {
                    errors += e; exchange.sendResponseHeaders(500, -1)
                }
            }
        }
        server.start()
        val url = "http://127.0.0.1:${server.address.port}"
        try {
            test(ReviewClient(url, url, "provider-secret", "provider-secret")); errors.firstOrNull()?.let { throw it }
        } finally {
            server.stop(0)
        }
    }

    private fun HttpExchange.query(): Map<String, String> =
        requestURI.rawQuery.orEmpty().split('&').filter(String::isNotEmpty).associate {
            URLDecoder.decode(it.substringBefore('='), Charsets.UTF_8) to URLDecoder.decode(
                it.substringAfter('='),
                Charsets.UTF_8
            )
        }

    private fun githubPr(number: Int = 7, merged: String = "2026-09-21T12:00:00Z") = """
        {"number":$number,"title":"Fix equality","body":"Full review context","html_url":"https://github.test/o/r/pull/$number",
        "merged_at":"$merged","updated_at":"2026-09-22T12:00:00Z","base":{"sha":"$before"},"head":{"sha":"$after"}}
    """.trimIndent()

    @Test
    fun `GitHub preserves complete paginated discussions with review context and bot filtering`() {
        var comments = 0
        var reviews = 0
        fixture({ exchange ->
            when (exchange.requestURI.path) {
                "/repos/o/r/pulls/7" -> githubPr()
                "/repos/o/r/pulls/7/reviews" -> {
                    reviews++
                    if (reviews == 1) {
                        exchange.responseHeaders.add("Link", "<https://untrusted.invalid/page>; rel=\"next\"")
                        """[{"id":90,"body":"Complete overview","user":{"login":"reviewer"},"submitted_at":"2026-09-20T10:00:00Z"}]"""
                    } else "[]"
                }

                "/repos/o/r/pulls/7/comments" -> {
                    comments++
                    if (exchange.query()["page"] == "1") {
                        exchange.responseHeaders.add("Link", "<https://untrusted.invalid/page>; rel=\"next\"")
                        """[{"id":11,"body":${JsonPrimitive("complete root message ".repeat(100))},"path":"src/A.java","original_commit_id":"$before","original_start_line":2,"original_line":4,"pull_request_review_id":90,"user":{"login":"reviewer"},"created_at":"2026-09-20T11:00:00Z"}]"""
                    } else """[{"id":12,"in_reply_to_id":11,"body":"Fixed","user":{"login":"author"},"created_at":"2026-09-20T12:00:00Z"},{"id":13,"in_reply_to_id":11,"body":"noise","user":{"login":"helper[bot]","type":"Bot"}}]"""
                }

                else -> error("Unexpected provider path")
            }
        }) { client ->
            val pr = client.fetch(ReviewSelection("github", "o", "r", 1, listOf(7))).single()
            val thread = pr.threads.single()
            assertEquals(
                listOf("Complete overview", "complete root message ".repeat(100), "Fixed"),
                thread.messages.map { it.body })
            assertEquals(2, thread.anchorLine); assertEquals(4, thread.anchorEndLine)
            assertEquals(2, comments); assertEquals(2, reviews)
        }
    }

    @Test
    fun `GitHub date selection uses merge date and inclusive end despite recently updated old PR`() {
        fixture({ exchange ->
            when (exchange.requestURI.path) {
                "/repos/o/r/pulls" -> "[${githubPr(1, "2020-01-01T00:00:00Z")},${
                    githubPr(
                        2,
                        "2026-09-21T23:59:59Z"
                    )
                },${githubPr(3, "2026-09-22T00:00:00Z")}]"

                "/repos/o/r/pulls/2" -> githubPr(2)
                "/repos/o/r/pulls/2/comments", "/repos/o/r/pulls/2/reviews" -> "[]"
                else -> error("Selected wrong PR")
            }
        }) { client ->
            assertEquals(
                listOf(2),
                client.fetch(ReviewSelection("github", "o", "r", 10, startDate = "2026-09-21", endDate = "2026-09-21"))
                    .map { it.number })
        }
    }

    @Test
    fun `Space paginates feed and complete human discussion`() {
        var feeds = 0
        var discussions = 0
        fixture({ exchange ->
            when (exchange.requestURI.path) {
                "/api/http/projects/key:O/code-reviews/number:7" -> """{"id":"review-id","number":7,"state":"Closed","timestamp":1790000000000,"title":"Fix equality","description":"Context","feedChannelId":"feed","branchPair":{"repository":"repo","isMerged":true,"sourceBranchRef":"$after","targetBranchInfo":{"ref":"$before"}}}"""
                "/api/http/chats/messages/sync-batch" -> {
                    feeds++
                    if (feeds == 1) """{"data":[],"etag":"second","hasMore":true}""" else {
                        assertContains(exchange.query().getValue("batchInfo"), "etag:second")
                        """{"data":[{"chatMessage":{"id":"root","projectedItem":{"author":{"name":"Reviewer","details":{"user":{"id":"person"}}}},"details":{"className":"CodeDiscussionAddedFeedEvent","codeDiscussion":{"id":"discussion","channel":{"id":"thread"},"anchor":{"filename":"/src/A.java","line":3,"revision":"$before"}}}}}],"etag":"done","hasMore":false}"""
                    }
                }

                "/api/http/chats/messages" -> {
                    discussions++
                    val ids = if (discussions == 1) 1..50 else 51..51
                    val messages =
                        ids.joinToString(",") { """{"id":"$it","text":"Complete message $it","author":{"name":"Reviewer"},"created":{"iso":"2026-09-21T10:00:00Z"}}""" }
                    """{"messages":[$messages],"nextStartFromDate":{"timestamp":1790000000000},"orgLimitReached":false}"""
                }

                else -> error("Unexpected Space path")
            }
        }) { client ->
            val thread = client.fetch(ReviewSelection("space", "O", "repo", 1, listOf(7))).single().threads.single()
            assertEquals(51, thread.messages.size)
            assertEquals("Complete message 51", thread.messages.last().body)
            assertEquals("src/A.java", thread.filePath)
            assertEquals(2, feeds); assertEquals(2, discussions)
        }
    }

    @Test
    fun `snapshot reads reject truncation and Git creates complete canonical diff`() {
        var truncate = false
        fixture({ exchange ->
            val content = if (exchange.query()["ref"] == before) beforeSource else afterSource
            val encoded = Base64.getEncoder().encodeToString(content.toByteArray())
            """{"type":"file","encoding":"base64","content":"$encoded","size":${content.toByteArray().size + if (truncate) 1 else 0}}"""
        }) { client ->
            val repo = ReviewRepository("github", "o", "r")
            val diff = client.diff(repo, before, after, fixturePath, fixturePath)
            assertEquals(listOf(3), UnifiedDiff.parse(diff).before[fixturePath])
            assertContains(diff, "+    return java.util.Objects.equals(a, b);")
            truncate = true
            assertFails { client.file(repo, before, fixturePath) }
        }
    }

    @Test
    fun `PR validation binds ordered coverage and exact bytes to coordinator task`() {
        val signal = workspace.signals().first()
        val review = PullRequest(
            7,
            "https://review/7",
            "Fix equality",
            "Context",
            signal.source.parentRevision!!,
            signal.source.commitRevision!!,
            1790000000000,
            listOf(
                ReviewThread(
                    "root",
                    "https://review/7#root",
                    historyPath,
                    signal.fileRevision.revision,
                    5,
                    5,
                    listOf(ReviewMessage("reviewer", "Compare string values", "2026-09-21"))
                )
            )
        )
        val provider = object : ReviewProvider {
            override fun fetch(selection: ReviewSelection) = listOf(review)
            override fun file(repository: ReviewRepository, revision: String, path: String) =
                error("Unexpected source call")

            override fun diff(
                repository: ReviewRepository,
                before: String,
                after: String,
                beforePath: String,
                afterPath: String
            ) = error("Unexpected diff call")
        }
        Store(workspace.state).use { store ->
            val analysis = PrAnalysis(store, provider)
            val manager = store.createPlan(
                "Reviews",
                listOf(Step("edict-pr-signal-analysis", "First"), Step("edict-pr-signal-analysis", "Second"))
            )
            val coordinator = store.launch(
                manager.token,
                manager.plan.tasks[0].id,
                "edict-pr-signal-analysis",
                listOf("inbox.write"),
                listOf("inbox")
            )
            val other = store.launch(
                manager.token,
                manager.plan.tasks[1].id,
                "edict-pr-signal-analysis",
                listOf("inbox.write"),
                listOf("inbox")
            )
            assertFails { store.finishTask(coordinator.token, "completed", "Premature") }
            val batch = analysis.prepare(coordinator.token, ReviewSelection("github", "o", "r", 1, listOf(7)))
            assertFails { analysis.list(other.token, batch.batchId, 0, 20) }
            val item = analysis.list(coordinator.token, batch.batchId, 0, 20).items.single()
            val prSignal = signal.copy(
                source = SignalSource(
                    "FromPR", signal.source.diffPositiveToNegative, prNumber = 7, title = review.title,
                    discussionMessages = listOf("Compare string values"), url = "https://review/7#root"
                ), provenance = Provenance(item.workItemId)
            )
            val content = json.encodeToString(prSignal)
            val path = "inbox/${signal.id}.json"
            assertFails { store.write(coordinator.token, path, content, "") }
            assertFails { analysis.validate(coordinator.token, batch.batchId, emptyList(), listOf(content)) }
            assertFails {
                analysis.validate(
                    coordinator.token,
                    batch.batchId,
                    listOf(item.workItemId),
                    listOf(json.encodeToString(prSignal.copy(source = prSignal.source.copy(title = "Invented"))))
                )
            }
            analysis.validate(coordinator.token, batch.batchId, listOf(item.workItemId), listOf(content))
            assertFails { store.finishTask(coordinator.token, "completed", "Missing publication") }
            assertFails { store.write(coordinator.token, path, "$content\n", "") }
            store.write(coordinator.token, path, content, "")
            store.finishTask(coordinator.token, "completed", "Published")
            assertFails { analysis.get(coordinator.token, batch.batchId, item.workItemId) }
        }
    }
}
