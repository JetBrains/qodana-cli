// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import io.github.oshai.kotlinlogging.KotlinLogging
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.common.GitRepository
import org.jetbrains.qodana.edict.signals.validRevision
import java.nio.file.Path
import java.util.Properties
import kotlin.io.path.inputStream

private val logger = KotlinLogging.logger {}

/** The commit authors that are bots, as the email addresses of their commits tell. */
private val BOT_EMAIL = Regex("patronus|no-reply|noreply", RegexOption.IGNORE_CASE)

/** The branch TeamCity builds a SafePush on, named after its user and the id Patronus gives it. */
private val SAFE_PUSH_BRANCH = Regex("refs/patronus/.+-([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})/checks")

/** The TeamCity build of the SafePush [patronusId]: its commits start after [baseCommit], and [buildUrl] shows the build. */
internal data class SafePushBuild(val patronusId: String, val baseCommit: String, val buildUrl: String)

/**
 * The newest commit of a person that a SafePush pushes, [revision], whose author [email] gets its feedback, and whose [subject]
 * may title its review. A SafePush may end in a commit of Patronus that merges the target branch in.
 */
internal data class SafePushAuthor(val revision: String, val email: String, val subject: String)

/** A SafePush that a TeamCity [build] analyses, made by [author], and the [review] it is of. */
internal data class SafePush(val build: SafePushBuild, val author: SafePushAuthor, val review: ReviewReference)

/**
 * The SafePush that the TeamCity build of this process analyses in [project], a checkout of [repository], with the review it
 * is of, as [reviews] find it; none when the build is of no SafePush or only bots made its commits.
 */
internal fun analysedSafePush(repository: ReviewRepository, project: Path, reviews: SpaceSafePushReviews): SafePush? {
    val properties = System.getenv("TEAMCITY_BUILD_PROPERTIES_FILE")?.takeIf(String::isNotBlank)
        ?: error("TEAMCITY_BUILD_PROPERTIES_FILE is not set; Edict feedback on Space runs in a TeamCity build")
    val build = teamCitySafePushBuild(Path.of(properties)) ?: return null
    val author = safePushAuthor(project, build.baseCommit)
    if (author == null) {
        logger.info { "No person made the commits of SafePush ${build.patronusId}; no Edict feedback" }
        return null
    }
    val review = reviews.review(author, build.patronusId)
    logger.info { "SafePush ${build.patronusId} of ${author.email} in ${repository.label()} gets Edict feedback as ${review.url}" }
    return SafePush(build, author, review)
}

/**
 * The SafePush that a TeamCity build analyses, as the parameters TeamCity writes for each build step tell it: the system
 * properties in [propertiesFile], which `TEAMCITY_BUILD_PROPERTIES_FILE` names, and the configuration parameters in the file
 * those name. TeamCity writes them again for each step, so they hold the values earlier steps set. None, after a warning, when
 * the build is of another branch.
 */
internal fun teamCitySafePushBuild(propertiesFile: Path): SafePushBuild? {
    val parameters = TeamCityParameters.read(propertiesFile)
    val branch = parameters.required("teamcity.build.branch")
    val patronusId = SAFE_PUSH_BRANCH.matchEntire(branch)?.groupValues?.get(1)
    if (patronusId == null) {
        logger.warn { "$FEEDBACK_UNAVAILABLE: the build is of $branch, which is no SafePush" }
        return null
    }
    val baseCommit = parameters.required("baseCommit")
    check(validRevision(baseCommit)) { "The TeamCity parameter baseCommit is no full commit SHA: '$baseCommit'" }
    val server = parameters.required("teamcity.serverUrl").trimEnd('/')
    val buildUrl = "$server/buildConfiguration/${parameters.required("teamcity.buildType.id")}/${parameters.required("teamcity.build.id")}"
    return SafePushBuild(patronusId, baseCommit, buildUrl)
}

/**
 * The newest commit of a person among those [project] has on top of [baseCommit], following first parents, as the SafePush
 * builds have always found the author to notify; none when bots made them all.
 */
internal fun safePushAuthor(project: Path, baseCommit: String): SafePushAuthor? =
    GitRepository(project).git("log", "--first-parent", "--format=%H%x00%ae%x00%s", "$baseCommit..HEAD").lines()
        .filter(String::isNotEmpty)
        .map { it.split('\u0000', limit = 3) }
        .firstOrNull { !BOT_EMAIL.containsMatchIn(it[1]) }
        ?.let { (revision, email, subject) -> SafePushAuthor(revision, email, subject) }

/** The parameters of a TeamCity build step: its [system] properties and its [configuration] parameters. */
private class TeamCityParameters(private val system: Properties, private val configuration: Properties) {
    /** The value of the parameter [name], a configuration parameter or else a system property; fails when it has none. */
    fun required(name: String): String =
        (configuration.getProperty(name) ?: system.getProperty(name))?.takeIf(String::isNotBlank)
            ?: error("The TeamCity build has no parameter $name")

    companion object {
        /** The parameters whose system properties are in [file]. */
        fun read(file: Path): TeamCityParameters {
            val system = properties(file)
            val configuration = system.getProperty("teamcity.configuration.properties.file")?.takeIf(String::isNotBlank)
                ?: error("$file names no file of the configuration parameters")
            return TeamCityParameters(system, properties(Path.of(configuration)))
        }
    }
}

private fun properties(file: Path): Properties = Properties().apply { file.inputStream().use(::load) }
