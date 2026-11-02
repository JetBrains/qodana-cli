// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.promotion

import org.jetbrains.qodana.edict.ci.ReviewApi
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.edictnext.PromotionPrState

/**
 * Fails when an active promotion's review is outside the configured promotion repository, which could never check it.
 * Active are inspection promotions on review and weak-signal reviews in `promotions/active.json`; resolved ones are
 * history and may belong to an earlier repository.
 */
internal fun requireActivePromotionsOwnedBy(repository: EdictRepository, reviews: ReviewApi?) {
  val inspections = repository.loadClusters().flatMap { cluster ->
    cluster.manifest.promotions.filter { it.state == PromotionPrState.ON_REVIEW }.map { cluster.id to it.pullRequest }
  }
  val weakSignals = repository.loadActiveWeakSignalPromotions().flatMap { (clusterId, promotions) ->
    promotions.map { clusterId to it.pullRequest }
  }
  val foreign = (inspections + weakSignals).filterNot { (_, reference) -> reviews?.repository?.owns(reference) == true }
  require(foreign.isEmpty()) {
    val configured = reviews?.repository?.label()
      ?: "no promotion repository (edict.promotion.url or edict.ci.url)"
    "Active promotions belong to another repository than the configured $configured. Restore that configuration " +
      "until they are resolved: " +
      foreign.joinToString("; ") { (clusterId, reference) -> "cluster '$clusterId' ${reference.reviewRepository.label()} ${reference.url}" }
  }
}
