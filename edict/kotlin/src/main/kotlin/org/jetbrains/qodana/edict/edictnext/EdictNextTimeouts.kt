package org.jetbrains.qodana.edict.edictnext

import kotlin.time.Duration
import kotlin.time.Duration.Companion.minutes

/** Runtime bounds enforced by Edict Next. */
internal object EdictNextTimeouts {
  /** The whole agent session. */
  val session: Duration
    get() = property("qodana.edictnext.timeout.minutes") ?: 900.minutes

  private fun property(name: String): Duration? {
    val value = System.getProperty(name) ?: return null
    val minutes = value.toLongOrNull()
    require(minutes != null) { "System property $name must be a whole number of minutes, got '$value'" }
    return minutes.minutes
  }
}
