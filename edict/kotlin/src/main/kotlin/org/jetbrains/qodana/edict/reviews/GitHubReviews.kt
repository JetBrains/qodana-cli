// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.reviews

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.number
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneOffset

private fun ReviewClient.githubPages(endpoint: String): List<JsonObject> {
    val all = mutableListOf<JsonObject>()
    for (page in 1..1000) {
        val response = get("github", endpoint, mapOf("per_page" to "100", "page" to "$page"))
        val items = response.body.jsonArray.map { it.jsonObject }
        all += items
        if (!response.hasNext && items.size < 100) return all
        require(items.isNotEmpty()) { "GitHub pagination advertised another page without items" }
    }
    error("GitHub pagination exceeded 1000 pages; refusing incomplete evidence")
}

internal fun ReviewClient.github(selection: ReviewSelection): List<PullRequest> {
    val root = endpoint(selection.repository)
    val numbers = selection.prNumbers.toMutableList()
    if (numbers.isEmpty()) {
        val start = LocalDate.parse(selection.startDate).atStartOfDay().toInstant(ZoneOffset.UTC)
        val seen = mutableSetOf<Int>()
        for (page in 1..1000) {
            val response = get(
                "github",
                "$root/pulls",
                mapOf(
                    "state" to "closed",
                    "sort" to "updated",
                    "direction" to "desc",
                    "per_page" to "100",
                    "page" to "$page"
                )
            )
            val items = response.body.jsonArray.map { it.jsonObject }
            var old = false
            for (item in items) {
                val number = item.number("number").toInt()
                require(number > 0 && seen.add(number)) { "Invalid or repeated PR number" }
                if (Instant.parse(item.text("updated_at")).isBefore(start)) {
                    old = true; break
                }
                if (item.text("merged_at").isNotEmpty() && selection.containsDate(
                        Instant.parse(item.text("merged_at")).toEpochMilli()
                    )
                ) numbers += number
                if (numbers.size == selection.maxPrs) break
            }
            if (old || numbers.size == selection.maxPrs || !response.hasNext && items.size < 100) break
            require(items.isNotEmpty() && page < 1000) { "Incomplete GitHub PR discovery" }
        }
    }
    return numbers.mapNotNull { number ->
        val endpoint = "$root/pulls/$number"
        val v = get("github", endpoint).body.jsonObject
        require(v.number("number").toInt() == number) { "GitHub returned wrong PR" }
        if (v.text("merged_at").isEmpty()) return@mapNotNull null
        val merged = Instant.parse(v.text("merged_at")).toEpochMilli()
        if (!selection.containsDate(merged)) return@mapNotNull null
        val pr = PullRequest(
            number,
            v.text("html_url"),
            v.text("title"),
            v.text("body"),
            v.obj("base").text("sha"),
            v.obj("head").text("sha"),
            merged
        )
        val comments = githubPages("$endpoint/comments")
        val reviews = githubPages("$endpoint/reviews")
        val byId = comments.associateBy { it.number("id") }
        require(byId.size == comments.size && byId.keys.all { it > 0 }) { "Invalid or duplicate comment IDs" }
        val threads = comments.groupBy { it.number("in_reply_to_id").takeIf { id -> id > 0 } ?: it.number("id") }
            .mapNotNull { (id, group) ->
                val anchor = byId[id] ?: error("Missing root comment $id")
                val line = anchor.number("original_line").takeIf { it > 0 } ?: anchor.number("line")
                val start = anchor.number("original_start_line").takeIf { it > 0 } ?: anchor.number("start_line")
                    .takeIf { it > 0 } ?: line
                val messages = reviews.filter {
                    it.number("id") == anchor.number("pull_request_review_id") && it.text("body").isNotBlank()
                }
                    .filterNot { isBot(it.obj("user").text("login"), it.obj("user").text("type")) }
                    .map { ReviewMessage(it.obj("user").text("login"), it.text("body"), it.text("submitted_at")) } +
                        group.sortedWith(compareBy({ it.text("created_at") }, { it.number("id") }))
                            .filterNot { isBot(it.obj("user").text("login"), it.obj("user").text("type")) }
                            .map { ReviewMessage(it.obj("user").text("login"), it.text("body"), it.text("created_at")) }
                if (messages.isEmpty()) null else ReviewThread(
                    "github-$number-$id",
                    "${pr.url}#discussion_r$id",
                    anchor.text("path"),
                    anchor.text("original_commit_id"),
                    start.toInt(),
                    line.toInt(),
                    messages
                )
            }.sortedWith(compareBy({ it.messages.first().createdAt }, { it.threadId }))
        pr.copy(threads = threads)
    }
}
