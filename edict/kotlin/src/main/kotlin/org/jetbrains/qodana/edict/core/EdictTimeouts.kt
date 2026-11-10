package org.jetbrains.qodana.edict.core

import kotlin.time.Duration
import kotlin.time.Duration.Companion.minutes

/** Runtime bounds enforced by Edict. */
internal object EdictTimeouts {
  /** The whole agent session. */
  val session: Duration
    get() = property("qodana.edict.timeout.minutes") ?: 900.minutes

  private fun property(name: String): Duration? {
    val value = System.getProperty(name) ?: return null
    val minutes = value.toLongOrNull()
    require(minutes != null) { "System property $name must be a whole number of minutes, got '$value'" }
    return minutes.minutes
  }
}
