package org.jetbrains.qodana.edict.edictnext

import io.modelcontextprotocol.kotlin.sdk.server.Server
import io.modelcontextprotocol.kotlin.sdk.server.ServerOptions
import io.modelcontextprotocol.kotlin.sdk.types.*
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.history.enrichment.EdictNextEnrichmentMcpToolset
import org.jetbrains.qodana.edict.promotion.PromotionReviewDecision
import org.jetbrains.qodana.edict.promotion.PromotionService
import org.jetbrains.qodana.edict.promotion.WeakSignalFix
import org.jetbrains.qodana.edict.ci.ReviewReference
import kotlin.coroutines.cancellation.CancellationException

/** Registers the Edict Next tools on the standalone MCP Kotlin SDK server. */
internal class EdictNextMcpToolset(
  private val layout: EdictLayout,
  private val inspectionServer: IntellijMcpServerService,
  private val management: EdictManagementService,
  private val distribution: EdictNextDistributionService,
  private val generation: EdictNextGenerationService,
  private val enrichment: EdictNextEnrichmentMcpToolset,
  private val sourceFiles: EdictSourceFileService,
  private val configuration: EdictConfiguration = EdictConfiguration(),
  private val promotion: PromotionService,
) {
  fun createServer(): Server = Server(
    serverInfo = Implementation(
      name = SERVER_NAME,
      version = "1.0.0",
    ),
    options = ServerOptions(
      capabilities = ServerCapabilities(
        tools = ServerCapabilities.Tools(listChanged = false),
      ),
    ),
    instructions = EdictManagementService.INSTRUCTIONS,
  ).also(::registerTools)

  private fun registerTools(server: Server) {
    management.registerTools(server, configuration.calculatePrice)
    enrichment.registerTools(server)
    registerPromotionTools(server)

    server.addTool(
      name = "edict_context",
      description = "Return this run's configured paths, the caller's task-owned scratch directory, CI review repository, and default extraction Signal target. Read-only.",
      inputSchema = toolSchema(
        required = listOf("token"),
        "token" to property("string", "Your managed capability token"),
      ),
    ) { request ->
      EdictRunContext(
        projectDirectory = layout.root.toString(),
        stateDirectory = layout.stateDirectory.toString(),
        scratchDirectory = management.scratchDirectory(request.requireString("token")).toString(),
        reviewRepository = configuration.ci?.reviewRepository,
        defaultSignalCount = configuration.extraction.defaultSignalCount,
      ).toToolResult()
    }

    server.addTool(
      name = "edict_file_at_ref",
      description = "Read a repository file at an exact commit with 1-based line numbers, from local Git or, when the " +
              "commit is not local, from the CI review repository. With anchor lines, return only them and radius lines around " +
              "them. Read-only.",
      inputSchema = toolSchema(
        required = listOf("path", "ref"),
        "path" to property("string", "File path relative to the repository root"),
        "ref" to property("string", "Full commit SHA"),
        "anchorStartLine" to property("integer", "Optional 1-based first line to focus on"),
        "anchorEndLine" to property(
          "integer",
          "Optional 1-based last line to focus on; defaults to anchorStartLine"
        ),
        "radius" to property(
          "integer",
          "Context lines around the anchor (default ${EdictSourceFileService.DEFAULT_RADIUS})"
        ),
      ),
    ) { request ->
      val anchor = request.optionalInt("anchorStartLine")
        ?.let { EdictNextLineRange(it, request.optionalInt("anchorEndLine") ?: it) }
      val content = sourceFiles.fileAtRef(
        request.requireString("path"),
        request.requireString("ref"),
        anchor,
        request.optionalInt("radius") ?: EdictSourceFileService.DEFAULT_RADIUS,
      )
      CallToolResult(content = listOf(TextContent(content)))
    }

    server.addTool(
      name = "edict_next_calculate_neighbours_by_embeddings",
      description = "Replace re-published clustered Signals, validate the state repository, and calculate the nearest embedding neighbours of every inbox Signal. Call once before edict_next_next_signal.",
    ) {
      distribution.calculateNeighbours().toToolResult()
    }

    server.addTool(
      name = "edict_next_next_signal",
      description = "Return the next alphabetical inbox Signal of this run and its nearest cluster or inbox candidates. " +
              "Needed only to start a task: each assignment returns the next Signal. Calling it again returns the same prepared Signal. " +
              "The nearest small clusters include their member Signals, which counts as loaded cluster context; inbox " +
              "candidates include their label, path and description. " +
              "The first call selects a batch capped by edict.distribution.defaultSignalCount from qodana.yaml. " +
              "Call until it returns STOP_DISTRIBUTION; the batch size cannot change after the first call. " +
              "After a task's share of the batch it returns HAND_OFF: finish that task, and a new task continues the batch.",
    ) {
      distribution.nextSignal().toToolResult()
    }

    server.addTool(
      name = "edict_next_get_distribution_context",
      description = "Return distribution evidence for one Signal or every member of a cluster. Cluster retrieval records evidence access for the current Signal.",
      inputSchema = stringArguments(
        "kind" to "Context kind: 'signal' or 'cluster'",
        "id" to "Signal or cluster id",
      ),
    ) { request ->
      distribution
        .getContext(request.requireString("kind"), request.requireString("id"))
        .toToolResult()
    }

    server.addTool(
      name = "edict_next_add_signal_to_cluster",
      description = "Add a prepared Signal to an existing cluster or create a new Pending cluster with the supplied id. Existing-cluster assignment requires its context to have been loaded for the current Signal. This is the only distribution mutation. " +
              "After an assignment, `next` holds what edict_next_next_signal would return: the next Signal, HAND_OFF or STOP_DISTRIBUTION.",
      inputSchema = stringArguments(
        "signalId" to "ID of the current prepared Signal",
        "clusterId" to "Existing or new kebab-case cluster id",
      ),
    ) { request ->
      val signalId = request.requireString("signalId")
      try {
        distribution
          .addSignalToCluster(signalId, request.requireString("clusterId"))
          .toToolResult()
      } catch (e: Exception) {
        EdictNextSignalValidationResponse(
          signalId = signalId,
          summary = e.message ?: "The ${e.javaClass} error occurred",
          added = false,
        ).toToolResult(isError = true)
      }
    }

    server.addTool(
      name = "edict_next_select_generation_clusters",
      description = "Suggest Pending clusters for generation, ranked by descending distinct source-file count among unprocessed strong positive Signals and requiring " +
              "strong positive evidence. Signals covered by a successful generation or inspection reuse do not count toward ranking; " +
              "re-published replacements count again. Read-only: creates no state or reservations; each call reflects the current repository. " +
              "The manager may choose other Pending clusters without calling this tool. Returns the concurrency limit as well. " +
              "Omit generationCount to use the configured default.",
      inputSchema = toolSchema(
        required = emptyList(),
        "generationCount" to buildJsonObject {
          put("type", "integer")
          put("minimum", 1)
          put(
            "description",
            "Optional number of clusters to select; omitted uses edict.generation.defaultGenerationCount"
          )
        },
      ),
    ) { request ->
      generation.selectGenerationClusters(request.optionalInt("generationCount")).toToolResult()
    }

    server.addTool(
      name = "edict_next_validate_code_example",
      description = "Validate one stored code example structurally: metadata, parsing, and target ranges. Whether the example is semantically correct stays the agent's decision.",
      inputSchema = stringArguments(
        "clusterId" to "Cluster id",
        "exampleId" to "Code example id inside the cluster",
      ),
    ) { request ->
      generation
        .validateCodeExample(request.requireString("clusterId"), request.requireString("exampleId"))
        .toToolResult()
    }

    server.addTool(
      name = "edict_next_save_code_example",
      description = "Persist one complete synthetic example in the read-only managed state. Code-example, overseer, and weak-review workers must use this instead of filesystem writes. " +
        "It first checks the example structurally, like edict_next_validate_code_example, and refuses an invalid one with its issues, so a stored example needs no separate validation.",
      inputSchema = stringArguments(
        "token" to "Your delegated managed-task token",
        "clusterId" to "Pending cluster chosen for generation",
        "exampleId" to "New or existing lowercase kebab-case example id",
        "metadataJson" to "Complete metadata JSON with id, fileName, label, and expectedRanges",
        "sourceCode" to "Complete self-contained Java or Kotlin source",
      ),
    ) { request ->
      val token = request.requireString("token")
      management.requireSkill(
        token,
        "edict-next-code-example",
        "edict-next-code-example-overseer",
        "edict-next-weak-signal-review",
        "edict-next-inspection-code-review",
      )
      management.requireTokenFree(request.requireString("metadataJson"))
      management.requireTokenFree(request.requireString("sourceCode"))
      generation.saveCodeExample(
        request.requireString("clusterId"),
        request.requireString("exampleId"),
        request.requireString("metadataJson"),
        request.requireString("sourceCode"),
      ).toToolResult()
    }

    server.addTool(
      name = "edict_next_update_code_example_expected_range",
      description = "Correct only the single expected line range of an existing positive example in a Pending cluster. " +
              "Generation workers only; source, labels, other metadata, and Signal assignments cannot change. " +
              "Verify the location from the evidence, record the correction in history, then validate the candidate again.",
      inputSchema = toolSchema(
        required = listOf("token", "clusterId", "exampleId", "startLine", "endLine"),
        "token" to property("string", "Your delegated managed-task token"),
        "clusterId" to property("string", "Pending cluster chosen for generation"),
        "exampleId" to property("string", "Existing positive example id in the cluster"),
        "startLine" to property("integer", "Corrected 1-based first expected line"),
        "endLine" to property("integer", "Corrected 1-based last expected line, inclusive"),
      ),
    ) { request ->
      management.requireSkill(request.requireString("token"), "edict-next-cluster-generation")
      require(request.arguments.orEmpty().keys.all { it in setOf("token", "clusterId", "exampleId", "startLine", "endLine") }) {
        "Only the expected range may be changed; unsupported arguments"
      }
      generation.updateCodeExampleExpectedRange(
        request.requireString("clusterId"),
        request.requireString("exampleId"),
        EdictNextLineRange(
          request.optionalInt("startLine") ?: error("'startLine' is required"),
          request.optionalInt("endLine") ?: error("'endLine' is required"),
        ),
      ).toToolResult()
    }

    server.addTool(
      name = "edict_next_assign_code_example",
      description = "Assign a structurally valid stored example to one persisted cluster Signal. Only syntheticExampleId is changed.",
      inputSchema = stringArguments(
        "token" to "Your delegated managed-task token",
        "clusterId" to "Pending cluster chosen for generation",
        "signalId" to "Persisted Signal id in the cluster",
        "exampleId" to "Validated example id in the cluster",
      ),
    ) { request ->
      management.requireSkill(
        request.requireString("token"),
        "edict-next-code-example",
        "edict-next-code-example-overseer",
      )
      generation.assignCodeExample(
        request.requireString("clusterId"),
        request.requireString("signalId"),
        request.requireString("exampleId"),
      ).toToolResult()
    }

    server.addTool(
      name = "edict_next_delete_code_example",
      description = "Delete one synthetic example from a Pending cluster and clear syntheticExampleId on every Signal that references it; those Signals then need a new example.",
      inputSchema = stringArguments(
        "token" to "Your delegated managed-task token",
        "clusterId" to "Pending cluster chosen for generation",
        "exampleId" to "Example id to delete",
      ),
    ) { request ->
      management.requireSkill(
        request.requireString("token"),
        "edict-next-code-example-overseer",
        "edict-next-weak-signal-review",
        "edict-next-inspection-code-review",
      )
      generation.deleteCodeExample(
        request.requireString("clusterId"),
        request.requireString("exampleId"),
      ).toToolResult()
    }

    server.addTool(
      name = "edict_next_validate_cluster_examples",
      description = "Validate that every cluster Signal has one structurally valid focused code example with a matching label.",
      inputSchema = stringArguments("clusterId" to "Cluster id"),
    ) { request ->
      generation
        .validateClusterExamples(request.requireString("clusterId"))
        .toToolResult()
    }

    server.addTool(
      name = "edict_next_get_cluster_examples",
      description = "Read every code example of a cluster in one answer: per example its label, whether a STRONG Signal is " +
              "assigned to it (strong) or not (weak), the Signals assigned to it, its file and expected ranges, then its " +
              "numbered source. Read-only.",
      inputSchema = stringArguments("clusterId" to "Cluster id"),
    ) { request ->
      val examples = try {
        generation.clusterExamples(request.requireString("clusterId"))
      } catch (e: EdictNextCodeExampleReadException) {
        "Cannot read every code example:\n" + e.issues.joinToString("\n")
      }
      CallToolResult(content = listOf(TextContent(examples)))
    }

    server.addTool(
      name = "edict_next_get_inspection_action",
      description = "Detect conflicting Signals, then return SKIP when the predecessor passes every strong example; otherwise return GENERATE. Call before changing the cluster id or inspection candidate.",
      inputSchema = stringArguments("clusterId" to "Cluster id"),
    ) { request ->
      generation
        .getInspectionAction(request.requireString("clusterId"))
        .toToolResult()
    }

    server.addTool(
      name = "edict_next_save_candidate_inspection",
      description = """Store the complete candidate Inspection KTS of a Pending cluster from a file in your scratch
        directory, then compile it, check its metadata against the cluster, and run every example. The candidate is stored
        even when it fails. Valid compilation, metadata and execution are required before project analysis; strong- and
        weak-example accuracy is advisory.""",
      inputSchema = stringArguments(
        "token" to "Your delegated edict-next-cluster-generation task token",
        "clusterId" to "Pending cluster chosen for generation",
        "inspectionKtsPath" to "Absolute path of the complete candidate Inspection KTS file below your scratch directory",
      ),
    ) { request ->
      val token = request.requireString("token")
      management.requireSkill(token, "edict-next-cluster-generation")
      val code = management.readScratchFile(token, request.requireString("inspectionKtsPath"))
      management.requireTokenFree(code)
      generation.saveCandidateInspection(request.requireString("clusterId"), code).toToolResult()
    }

    server.addTool(
      name = "edict_next_rename_cluster",
      description = """Rename a Pending cluster: its directory, cluster.json id, and candidate, including the
        candidate's `id = "<clusterId>"`. The predecessor keeps its id. Later calls use the new id.""",
      inputSchema = stringArguments(
        "token" to "Your delegated edict-next-cluster-generation task token",
        "clusterId" to "Pending cluster chosen for generation",
        "newClusterId" to "New lowercase kebab-case cluster id, unused by any cluster or inspection",
      ),
    ) { request ->
      management.requireSkill(request.requireString("token"), "edict-next-cluster-generation")
      generation.renameCluster(
        request.requireString("clusterId"),
        request.requireString("newClusterId"),
      ).toToolResult()
    }

    server.addTool(
      name = "edict_next_append_cluster_history",
      description = "Append one token-free operational decision to a Pending cluster history in managed state.",
      inputSchema = stringArguments(
        "token" to "Your delegated edict-next-cluster-generation task token",
        "clusterId" to "Pending cluster chosen for generation",
        "entry" to "Concise operational decision or known problem",
      ),
    ) { request ->
      management.requireSkill(request.requireString("token"), "edict-next-cluster-generation")
      management.requireTokenFree(request.requireString("entry"))
      generation.appendHistory(
        request.requireString("clusterId"),
        request.requireString("entry"),
      ).toToolResult()
    }

    server.addTool(
      name = "edict_next_get_new_inspection_results",
      description = """Run the current candidate, which must have passed validation when saved, over the analyzed project
        and create its weak-signal review manifest.
        Each cluster may run this a limited number of times per run; the response returns how many remain, and a call
        beyond the limit fails.""",
      inputSchema = stringArguments(
        "clusterId" to "Cluster id; the candidate is inspections/<clusterId>.candidate.kts",
        "privateScratchDirectory" to "Absolute private directory for review manifests and outputs",
      ),
    ) { request ->
      generation
        .getNewInspectionResults(
          request.requireString("clusterId"),
          request.requireString("privateScratchDirectory"),
        )
        .toToolResult()
    }

    server.addTool(
      name = "edict_next_record_weak_review",
      description = "Record how you classified findings of the cluster's last project analysis: one entry per finding with its path and line as the findings file gives them, its classification, and the example that represents it (the one you created, the existing one covering it, or the strong example of a contract mismatch). Only an example the analyzed inspection reports can represent a finding: an FP needs a reported NEGATIVE, a TP a reported POSITIVE; an example created after the analysis is run with the analyzed inspection now and must be reported too. A later round replaces the entry at the same location, which moves a finding to another example. The final evaluation weighs every example by the findings assigned to it.",
      inputSchema = stringArguments(
        "token" to "Your delegated edict-next-weak-signal-review task token",
        "clusterId" to "Pending cluster chosen for generation",
        "findingsJson" to "JSON array of {path, line, classification: TP|FP|UNCERTAIN, exampleId}; line is 0 for a finding without a line, and exampleId is required except for UNCERTAIN",
      ),
    ) { request ->
      management.requireSkill(request.requireString("token"), "edict-next-weak-signal-review")
      val findingsJson = request.requireString("findingsJson")
      management.requireTokenFree(findingsJson)
      generation.recordWeakReview(
        request.requireString("clusterId"),
        EdictNextJson.decodeFromString<List<EdictNextReviewedFinding>>(findingsJson),
      ).toToolResult()
    }

    server.addTool(
      name = "edict_next_record_evaluation",
      description = "Last step before finalising Generated: score the inspection that would be published (the analyzed candidate, or the predecessor on SKIP) on every strong and weak example and write clusters/<id>/evaluation.json: per-example results, tp, fp, fn, precision and recall weighted by the project findings the weak-signal review assigned to each example, and the published inspection's project findings. Records failing strong and weak examples too; accuracy does not block generation. Requires valid compilation, metadata and execution.",
      inputSchema = stringArguments(
        "token" to "Your delegated edict-next-cluster-generation task token",
        "clusterId" to "Pending cluster chosen for generation",
      ),
    ) { request ->
      management.requireSkill(request.requireString("token"), "edict-next-cluster-generation")
      generation.recordEvaluation(request.requireString("clusterId")).toToolResult()
    }

    server.addTool(
      name = "edict_next_finalise_cluster",
      description = """End the processing of a Pending cluster: record the reason in its history and move it to the
        requested status with that status's contract satisfied, or reject and leave the cluster unchanged.
        Generated: publishes the predecessor on SKIP, or the candidate that completed project analysis on GENERATE, when it
        has valid compilation, metadata and execution and edict_next_record_evaluation scored exactly it on the current examples.
        Strong- and weak-example accuracy never block Generated. Discontinued: removes the candidate and predecessor.
        Invalid: keeps the candidate and predecessor. Pending is not a generation outcome.""",
      inputSchema = toolSchema(
        listOf("token", "clusterId", "status", "reason"),
        "token" to property("string", "Your delegated edict-next-cluster-generation task token"),
        "clusterId" to property("string", "Pending cluster chosen for generation"),
        "status" to buildJsonObject {
          put("type", "string")
          put("description", "Status to set")
          putJsonArray("enum") { EdictNextClusterStatus.entries.filter { it != EdictNextClusterStatus.Pending }.forEach { add(it.name) } }
        },
        "reason" to property("string", "Token-free decision recorded in the cluster history"),
      ),
    ) { request ->
      val clusterId = request.requireString("clusterId")
      try {
        management.requireSkill(request.requireString("token"), "edict-next-cluster-generation")
        val reason = request.requireString("reason")
        management.requireTokenFree(reason)
        val status = EdictNextClusterStatus.valueOf(request.requireString("status"))
        generation.finaliseCluster(clusterId, status, reason).toToolResult()
      } catch (e: CancellationException) {
        throw e
      } catch (e: Exception) {
        EdictNextFinaliseClusterResponse(
          success = false,
          summary = e.message ?: "Failed to finalise cluster '$clusterId'",
        ).toToolResult(isError = true)
      }
    }

    registerInspectionKtsTools(server)
  }

  private fun registerPromotionTools(server: Server) {
    registerWeakSignalPromotionTools(server)
    server.addTool(
      name = "edict_promote_clusters",
      description = "Promote eligible Generated inspections with strong positive Signals from at least five distinct source files linked to source PRs or commit diffs. " +
        "Each PR includes a description and one representative positive Signal per source file. Omit clusterIds to process every eligible cluster.",
      inputSchema = toolSchema(
        required = listOf("token"),
        "token" to property("string", "Your delegated edict-promote task token"),
        "clusterIds" to buildJsonObject {
          put("type", "array")
          put("description", "Optional cluster IDs to consider; omitted means every cluster")
          put("minItems", 1)
          put("uniqueItems", true)
          putJsonObject("items") { put("type", "string") }
        },
      ),
    ) { request ->
      try {
        management.requireSkill(request.requireString("token"), "edict-promote")
        val clusterIds = request.arguments?.get("clusterIds")?.let {
          EdictNextJson.decodeFromJsonElement<List<String>>(it)
        }
        promotion.promote(clusterIds).toToolResult()
      } catch (e: Exception) {
        EdictNextMutationResponse(
          false,
          e.message ?: "Inspection promotion failed"
        ).toToolResult(isError = true)
      }
    }

    server.addTool(
      name = "edict-check-promotion",
      description = "Refresh ON_REVIEW promotion PRs, returning accepted promotions and closed-unmerged reviews awaiting a cluster decision.",
      inputSchema = toolSchema(
        required = listOf("token"),
        "token" to property("string", "Your delegated edict-check-promotion task token"),
      ),
    ) { request ->
      try {
        management.requireSkill(request.requireString("token"), "edict-check-promotion")
        val response = promotion.checkReviews()
        response.copy(
          undecided = response.undecided.map { it.copy(evidence = management.redact(it.evidence)) },
          failures = response.failures.map { it.copy(message = management.redact(it.message)) },
        ).toToolResult()
      } catch (e: Exception) {
        EdictNextMutationResponse(
          false,
          e.message ?: "Promotion review check failed"
        ).toToolResult(isError = true)
      }
    }

    val decisionObject = buildJsonObject {
      put("type", "object")
      putJsonObject("properties") {
        put("clusterId", property("string", "Cluster id"))
        put("promotionId", property("string", "Persisted promotion id"))
        put("decision", property("string", "MOVE_CLUSTER_TO_PENDING or DISCONTINUE_CLUSTER"))
        put("rationale", property("string", "Bounded decision rationale based on review evidence"))
      }
      putJsonArray("required") { listOf("clusterId", "promotionId", "decision", "rationale").forEach(::add) }
      put("additionalProperties", false)
    }
    server.addTool(
      name = "edict_promotion_decide_reviews",
      description = "Verify and resolve 1 to 10 closed reviews, atomically closing each promotion while updating cluster state and history.",
      inputSchema = toolSchema(
        required = listOf("token", "decisions"),
        "token" to property("string", "Your delegated edict-promotion-decision task token"),
        "decisions" to buildJsonObject {
          put("type", "array")
          put("minItems", 1)
          put("maxItems", 10)
          put("items", decisionObject)
        },
      ),
    ) { request ->
      try {
        management.requireSkill(request.requireString("token"), "edict-promotion-decision")
        val decisions = EdictNextJson.decodeFromJsonElement<List<PromotionReviewDecision>>(
          request.arguments?.get("decisions") ?: error("decisions is required"),
        )
        decisions.forEach { management.requireTokenFree(it.rationale) }
        val response = promotion.decideReviews(decisions)
        response.copy(
          failures = response.failures.map { it.copy(message = management.redact(it.message)) },
        ).toToolResult()
      } catch (e: Exception) {
        EdictNextMutationResponse(
          false,
          e.message ?: "Promotion review decision failed"
        ).toToolResult(isError = true)
      }
    }
  }

  private fun registerWeakSignalPromotionTools(server: Server) {
    server.addTool(
      name = "edict_check_weak_signal_promotion",
      description = "Refresh active weak-signal source-fix PR states. Return transient reviewer evidence, inspection digest and actual merged diffs. Independent of other signal extraction and inspection-promotion reviews.",
      inputSchema = toolSchema(
        required = listOf("token"),
        "token" to property("string", "Your delegated edict-check-weak-signal-promotion task token"),
      ),
    ) { request ->
      try {
        management.requireSkill(request.requireString("token"), "edict-check-weak-signal-promotion")
        val response = promotion.checkWeakSignalReviews(management::redact)
        response.copy(
          failures = response.failures.map { it.copy(message = management.redact(it.message)) },
        ).toToolResult()
      } catch (e: Exception) {
        EdictNextMutationResponse(
          false,
          e.message ?: "Weak-signal promotion review check failed",
        ).toToolResult(isError = true)
      }
    }

    server.addTool(
      name = "edict_record_weak_signal_promotion_signals",
      description = "Publish regular Signals for verified merged corrections already covered by the originating inspection. Direct publication marks them processed and requires an existing matching strong example. Other evidence must use edict_publish_signal to the inbox. Does not complete the review.",
      inputSchema = toolSchema(
        required = listOf("token", "clusterId", "pullRequest", "inspectionDigest", "signals"),
        "token" to property("string", "Your delegated edict-check-weak-signal-promotion task token"),
        "clusterId" to property("string", "Originating cluster returned by the check"),
        "pullRequest" to property("object", "Complete pullRequest reference returned by the check: provider, owner, repository, id, url"),
        "inspectionDigest" to property("string", "Unchanged inspection digest returned by the check"),
        "signals" to buildJsonObject { put("type", "array"); put("minItems", 1); put("items", management.signalSchema) },
      ),
    ) { request ->
      try {
        management.requireSkill(request.requireString("token"), "edict-check-weak-signal-promotion")
        val reference = EdictNextJson.decodeFromJsonElement<ReviewReference>(request.arguments!!.getValue("pullRequest"))
        val signals = EdictNextJson.decodeFromJsonElement<List<EdictNextSignal>>(request.arguments!!.getValue("signals"))
        signals.forEach { management.requireTokenFree(EdictNextJson.encodeToString(EdictNextSignal.serializer(), it)) }
        promotion.recordWeakSignalPromotionSignals(request.requireString("clusterId"), reference, request.requireString("inspectionDigest"), signals, management::redact).toToolResult()
      } catch (e: Exception) {
        EdictNextMutationResponse(false, management.redact(e.message ?: "Merged signal publication failed")).toToolResult(isError = true)
      }
    }

    server.addTool(
      name = "edict_complete_weak_signal_promotion",
      description = "Retire a merged weak-signal review from active checks after all actual evidence was published to the cluster/inbox, or analysis found no relevant correction. Never complete after a publication/evidence failure.",
      inputSchema = toolSchema(
        required = listOf("token", "clusterId", "pullRequest"),
        "token" to property("string", "Your delegated weak-promotion checker token"),
        "clusterId" to property("string", "Originating cluster"),
        "pullRequest" to property("object", "Complete review reference returned by the check"),
      ),
    ) { request ->
      try {
        management.requireSkill(request.requireString("token"), "edict-check-weak-signal-promotion")
        val reference = EdictNextJson.decodeFromJsonElement<ReviewReference>(request.arguments!!.getValue("pullRequest"))
        promotion.completeWeakSignalReview(request.requireString("clusterId"), reference)
        EdictNextMutationResponse(true, "Review completed").toToolResult()
      } catch (e: Exception) {
        EdictNextMutationResponse(false, management.redact(e.message ?: "Review completion failed")).toToolResult(isError = true)
      }
    }

    server.addTool(
      name = "edict_prepare_weak_signal_promotion",
      description = "Select up to five persisted true-positive project locations per Generated inspection that lacks referenced strong positive Signals from five distinct source files. Skips clusters with an active source-fix review, including merged reviews awaiting signal publication.",
      inputSchema = toolSchema(
        required = listOf("token"),
        "token" to property("string", "Your delegated edict-promote-weak-signal task token"),
        "clusterIds" to buildJsonObject {
          put("type", "array")
          put("minItems", 1)
          put("uniqueItems", true)
          putJsonObject("items") { put("type", "string") }
        },
      ),
    ) { request ->
      try {
        management.requireSkill(request.requireString("token"), "edict-promote-weak-signal")
        val clusterIds = request.arguments?.get("clusterIds")?.let { EdictNextJson.decodeFromJsonElement<List<String>>(it) }
        promotion.prepareWeakSignals(clusterIds).toToolResult()
      } catch (e: Exception) {
        EdictNextMutationResponse(false, e.message ?: "Weak-signal selection failed").toToolResult(isError = true)
      }
    }

    val fix = buildJsonObject {
      put("type", "object")
      putJsonObject("properties") {
        put("path", property("string", "Selected finding's repository-relative file path"))
        put("lines", buildJsonObject {
          put("type", "array")
          put("minItems", 1)
          put("maxItems", 5)
          put("uniqueItems", true)
          putJsonObject("items") { put("type", "integer"); put("minimum", 1) }
        })
        put("content", property("string", "Complete fixed file at the selected project revision"))
        put("explanation", property("string", "Why these changes repair the selected findings"))
      }
      putJsonArray("required") { listOf("path", "lines", "content", "explanation").forEach(::add) }
      put("additionalProperties", false)
    }
    server.addTool(
      name = "edict_promote_weak_signal",
      description = "Publish fixes for 1 to 5 selected true-positive locations in one source PR, describing the inspection and asking users to accept the improvements. Requires unchanged generated inspection and analyzed target revision.",
      inputSchema = toolSchema(
        required = listOf("token", "clusterId", "inspectionDigest", "fixes", "validation"),
        "token" to property("string", "Your delegated edict-promote-weak-signal task token"),
        "clusterId" to property("string", "Selected Generated cluster"),
        "inspectionDigest" to property("string", "Inspection digest returned by selection"),
        "fixes" to buildJsonObject {
          put("type", "array")
          put("minItems", 1)
          put("maxItems", 5)
          put("items", fix)
        },
        "validation" to property("string", "Validation performed, its results, and any checks that could not run"),
      ),
    ) { request ->
      try {
        management.requireSkill(request.requireString("token"), "edict-promote-weak-signal")
        val fixes = EdictNextJson.decodeFromJsonElement<List<WeakSignalFix>>(request.arguments?.get("fixes") ?: error("fixes is required"))
        val validation = request.requireString("validation")
        management.requireTokenFree(validation)
        fixes.forEach { management.requireTokenFree(it.content); management.requireTokenFree(it.explanation) }
        promotion.promoteWeakSignals(request.requireString("clusterId"), request.requireString("inspectionDigest"), fixes, validation).toToolResult()
      } catch (e: Exception) {
        EdictNextMutationResponse(false, e.message ?: "Weak-signal promotion failed").toToolResult(isError = true)
      }
    }
  }

  /**
   * Forwards the IntelliJ Inspection KTS tools for authoring and quick single-file checks, so agents only need this
   * server. Running examples and the whole project stays with the managed tools, which validate and bound them.
   */
  private fun registerInspectionKtsTools(server: Server) {
    fun proxy(name: String, description: String, inputSchema: ToolSchema) {
      server.addTool(name = name, description = description, inputSchema = inputSchema) { request ->
        val arguments = request.arguments ?: JsonObject(emptyMap())
        inspectionServer.withClient { it.callTool(name, arguments) }.toProxiedToolResult()
      }
    }

    val inspectionKtsCode = "inspectionKtsCode" to property("string", "The complete inspection.kts script")

    proxy(
      name = "generate_psi_tree",
      description = "Parse a Java or Kotlin code snippet with the IntelliJ PSI parser and return its PSI tree.",
      inputSchema = toolSchema(
        required = listOf("code", "language"),
        "code" to property("string", "Source code snippet to parse"),
        "language" to property("string", "Programming language: 'Java' or 'Kotlin'"),
      ),
    )
    proxy(
      name = "generate_inspection_kts_examples",
      description = "Return example inspection.kts scripts to use as templates for writing new inspections.",
      inputSchema = toolSchema(
        required = emptyList(),
        "language" to property("string", "Target language for examples: 'Java', 'Kotlin', or 'Any' (default)"),
        "includeAdditionalExamples" to property(
          "boolean",
          "If true, includes additional curated examples besides templates"
        ),
      ),
    )
    proxy(
      name = "generate_inspection_kts_api",
      description = "Return the inspection.kts API (PSI classes and helpers) available for the given language.",
      inputSchema = toolSchema(
        required = listOf("language"),
        "language" to property("string", "Target language: 'Java' or 'Kotlin'"),
        "wrapInTags" to property("boolean", "If true, wraps the API content in <API> and <api.kt> tags"),
      ),
    )
    proxy(
      name = "compile_inspection_kts",
      description = "Compile an inspection.kts script and return its inspection metadata without executing it.",
      inputSchema = toolSchema(required = listOf("inspectionKtsCode"), inspectionKtsCode),
    )
    proxy(
      name = "run_inspection_kts",
      description = "Compile an inspection.kts script and run it against one file of the inspected project, " +
              "optionally replacing its content. Returns compilation errors or the found problems.",
      inputSchema = toolSchema(
        required = listOf("inspectionKtsCode", "contextPath"),
        inspectionKtsCode,
        "contextPath" to property(
          "string",
          "Path of the target file relative to the inspected project, e.g. 'src/my/Example.kt'"
        ),
        "targetFileContent" to property(
          "string",
          "Content to analyze instead of the file on disk; the file must exist when omitted"
        ),
      ),
    )
  }

  companion object {
    /** The server's name in the MCP handshake, also the name agents register it under. */
    const val SERVER_NAME = "edict-mcp"
  }
}

private fun JsonObject.toProxiedToolResult(): CallToolResult = CallToolResult(
  content = (this["content"] as? JsonArray).orEmpty().mapNotNull { item ->
    val block = item as? JsonObject ?: return@mapNotNull null
    if (block["type"]?.jsonPrimitive?.content == "text") {
      TextContent(block["text"]?.jsonPrimitive?.content.orEmpty())
    } else null
  },
  isError = this["isError"]?.jsonPrimitive?.content?.toBooleanStrictOrNull(),
  structuredContent = this["structuredContent"] as? JsonObject,
)
