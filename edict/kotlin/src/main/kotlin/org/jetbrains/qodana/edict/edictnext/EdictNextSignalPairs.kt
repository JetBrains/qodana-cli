package org.jetbrains.qodana.edict.edictnext

import java.util.concurrent.ConcurrentHashMap

/** PR correction pairs stored in this server run; not persisted, so after a restart each Signal comes alone. */
internal class EdictNextSignalPairs {
  private val siblings = ConcurrentHashMap<String, String>()

  fun record(first: String, second: String) {
    siblings[first] = second
    siblings[second] = first
  }

  fun sibling(signalId: String): String? = siblings[signalId]
}
