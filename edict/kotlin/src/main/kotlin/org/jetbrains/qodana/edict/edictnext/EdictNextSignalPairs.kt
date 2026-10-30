package org.jetbrains.qodana.edict.edictnext

import java.util.concurrent.ConcurrentHashMap

/**
 * The POSITIVE and NEGATIVE Signals of one PR correction, as PR extraction stored them in this server run, so that
 * distribution can offer them together. Nothing persists it: after a restart distribution sees each Signal alone.
 */
internal class EdictNextSignalPairs {
  private val siblings = ConcurrentHashMap<String, String>()

  fun record(first: String, second: String) {
    siblings[first] = second
    siblings[second] = first
  }

  fun sibling(signalId: String): String? = siblings[signalId]
}
