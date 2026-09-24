// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.reviews

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.*
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneOffset

internal fun ReviewClient.space(selection: ReviewSelection): List<PullRequest> {
    val root = "/projects/key:${urlPart(selection.owner)}/code-reviews"
    val numbers = selection.prNumbers.toMutableList()
    if (numbers.isEmpty()) {
        val start = LocalDate.parse(selection.startDate).atStartOfDay().toInstant(ZoneOffset.UTC).toEpochMilli()
        val seen = mutableSetOf<Int>()
        for (page in 0 until 1000) {
            val response = get(
                "space", root, mapOf(
                    "\$skip" to "${page * 100}",
                    "\$top" to "100",
                    "state" to "Merged",
                    "repository" to selection.repo,
                    "sort" to "LastUpdatedDesc",
                    "to" to selection.endDate,
                    "\$fields" to "data(review(id,number,timestamp))"
                )
            ).body.jsonObject
            val items = response.array("data").map { it.obj("review") }
            var old = false
            for (item in items) {
                val number = item.number("number").toInt()
                require(number > 0 && seen.add(number) && item.number("timestamp") > 0) { "Invalid or repeated Space review" }
                if (item.number("timestamp") < start) {
                    old = true; break
                }
                if (selection.containsDate(item.number("timestamp"))) numbers += number
                if (numbers.size == selection.maxPrs) break
            }
            if (old || numbers.size == selection.maxPrs || items.size < 100) break
            require(page < 999) { "Incomplete Space review discovery" }
        }
    }
    return numbers.mapNotNull { number ->
        val v = get(
            "space",
            "$root/number:$number",
            mapOf("\$fields" to "id,number,state,timestamp,title,description,feedChannelId,branchPair(repository,isMerged,sourceBranchRef,targetBranchInfo(ref))")
        ).body.jsonObject
        require(
            v.number("number").toInt() == number && v.text("state").isNotEmpty()
        ) { "Wrong Space review or missing merge state" }
        if (v.text("state") != "Closed") return@mapNotNull null
        val pair = v.obj("branchPair")
        require(pair.text("repository") == selection.repo && pair.flag("isMerged") != null) { "Invalid Space review repository or merge status" }
        if (pair.flag("isMerged") != true || !selection.containsDate(v.number("timestamp"))) return@mapNotNull null
        val feed = v.text("feedChannelId")
        require(feed.isNotEmpty()) { "Space review lacks discussion feed" }
        val threads = spaceFeed(feed).mapNotNull { message ->
            val details = message.obj("details")
            val author = message.obj("projectedItem").obj("author")
            if (details.text("className") != "CodeDiscussionAddedFeedEvent" || author.obj("details").obj("user")
                    .isEmpty() || isBot(author.text("name"), author.obj("details").text("className"))
            ) return@mapNotNull null
            val discussion = details.obj("codeDiscussion")
            val channel = discussion.obj("channel").text("id")
            require(channel.isNotEmpty()) { "Discussion lacks channel or anchor" }
            val messages = spaceDiscussion(channel)
            if (messages.isEmpty()) return@mapNotNull null
            val anchor = discussion.obj("anchor")
            val line = (anchor.number("line").takeIf { it > 0 } ?: anchor.number("oldLine")).toInt()
            ReviewThread(
                "space-$number-${message.text("id")}",
                "$spaceUrl/im/review/${urlPart(v.text("id"))}?message=${urlPart(message.text("id"))}&channel=${
                    urlPart(feed)
                }",
                anchor.text("filename").removePrefix("/"),
                anchor.text("revision"),
                line,
                line,
                messages
            )
        }.sortedWith(compareBy({ it.messages.first().createdAt }, { it.threadId }))
        PullRequest(
            number, "$spaceUrl/p/${urlPart(selection.owner)}/reviews/$number", v.text("title"), v.text("description"),
            pair.obj("targetBranchInfo").text("ref"), pair.text("sourceBranchRef"), v.number("timestamp"), threads
        )
    }
}

private fun ReviewClient.spaceFeed(channel: String): List<JsonObject> {
    val all = linkedMapOf<String, JsonObject>()
    var etag = "0"
    val cursors = mutableSetOf(etag)
    for (page in 0 until 1000) {
        val response = get(
            "space", "/chats/messages/sync-batch", mapOf(
                "channel" to "id:$channel", "batchInfo" to "{etag:$etag,batchSize:100}",
                "\$fields" to "data(chatMessage(id,text,created,details(className,codeDiscussion(id,channel(id),anchor(filename,line,oldLine,revision))),projectedItem(author(name,details(className,user(id)))))),etag,hasMore"
            )
        ).body.jsonObject
        response.array("data").map { it.obj("chatMessage") }.filter { it.isNotEmpty() }
            .forEach { all.putIfAbsent(it.text("id"), it) }
        require(response.flag("hasMore") != null) { "Space feed omitted pagination completeness flag" }
        if (response.flag("hasMore") == false) return all.values.toList()
        etag = response.text("etag")
        require(etag.isNotEmpty() && cursors.add(etag)) { "Space feed pagination did not advance" }
    }
    error("Space feed exceeds 1000 pages; refusing incomplete evidence")
}

private fun ReviewClient.spaceDiscussion(channel: String): List<ReviewMessage> {
    val all = mutableListOf<ReviewMessage>()
    val seen = mutableSetOf<String>()
    val cursors = mutableSetOf<String>()
    var start = ""
    for (page in 0 until 1000) {
        val query = mutableMapOf(
            "channel" to "id:$channel", "sorting" to "FromOldestToNewest", "batchSize" to "50",
            "\$fields" to "messages(id,text,author(name,details(className,user(id))),created),nextStartFromDate,orgLimitReached"
        )
        if (start.isNotEmpty()) query["startFromDate"] = start
        val response = get("space", "/chats/messages", query).body.jsonObject
        require(response.flag("orgLimitReached") == false) { "Space discussion history unavailable or limited" }
        val messages = response.array("messages")
        var fresh = 0
        messages.forEach { message ->
            require(message.text("id").isNotEmpty()) { "Missing Space message ID" }
            if (!seen.add(message.text("id"))) return@forEach
            fresh++
            val author = message.obj("author")
            if (message.text("text").isNotEmpty() && !isBot(
                    author.text("name"),
                    author.obj("details").text("className")
                )
            ) {
                val created = message.obj("created")
                val timestamp =
                    created.text("iso").ifEmpty { Instant.ofEpochMilli(created.number("timestamp")).toString() }
                all += ReviewMessage(author.text("name"), message.text("text"), timestamp)
            }
        }
        if (messages.size < 50) return all
        val timestamp = response.obj("nextStartFromDate").number("timestamp")
        require(fresh > 0 && timestamp > 0) { "Space discussion pagination did not advance" }
        start = Instant.ofEpochMilli(timestamp).toString()
        require(cursors.add(start)) { "Space discussion repeated date cursor" }
    }
    error("Space discussion exceeds 1000 pages; refusing incomplete evidence")
}
