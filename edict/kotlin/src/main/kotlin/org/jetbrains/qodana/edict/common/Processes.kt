// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.common

import java.nio.file.Path
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

/** Argument-array execution only: repository evidence is never interpolated into a shell. */
internal fun runProcess(
    directory: Path,
    arguments: List<String>,
    timeoutSeconds: Long = 60,
    maxBytes: Int = 16 * 1024 * 1024
): String {
    val process = ProcessBuilder(arguments).directory(directory.toFile()).start()
    process.outputStream.close()
    val executor = Executors.newFixedThreadPool(2)
    try {
        fun read(stream: java.io.InputStream) = stream.use {
            val bytes = it.readNBytes(maxBytes + 1)
            if (bytes.size > maxBytes) {
                process.destroyForcibly(); error("Process output exceeded $maxBytes bytes; refusing truncated evidence")
            }
            bytes.toString(Charsets.UTF_8)
        }

        val out = executor.submit<String> { read(process.inputStream) }
        val err = executor.submit<String> { read(process.errorStream) }
        check(process.waitFor(timeoutSeconds, TimeUnit.SECONDS)) { "Process timed out after $timeoutSeconds seconds" }
        val stdout = out.get()
        val stderr = err.get()
        check(process.exitValue() == 0) { "${arguments.first()} exited ${process.exitValue()}: ${stderr.take(2000)}" }
        return stdout
    } finally {
        if (process.isAlive) {
            process.descendants().forEach { it.destroyForcibly() }
            process.destroyForcibly()
        }
        executor.shutdownNow()
    }
}
