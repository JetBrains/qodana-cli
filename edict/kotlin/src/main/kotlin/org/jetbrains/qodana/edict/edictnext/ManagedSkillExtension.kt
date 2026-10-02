// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.edictnext

import io.modelcontextprotocol.kotlin.sdk.server.Server
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.SignalPolicy
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Task

/**
 * Runtime policy and tools a service adds to the tasks of one managed skill. [EdictManagementService] applies it to
 * that skill's tasks only; its hooks run under the state lock.
 */
internal interface ManagedSkillExtension {
  val skill: String

  /** Registers this extension's tools through [management], which checks capability tokens and logs calls. */
  fun registerTools(server: Server, management: EdictManagementService) {}

  /** Non-null only if tasks of [skill] may publish inbox Signals; it rejects any they must not. */
  val signalPolicy: SignalPolicy? get() = null

  /** Rejects completing a task of [skill]. */
  fun beforeCompletion(task: Task) {}
}
