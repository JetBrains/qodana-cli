// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.transport.support

import com.sun.net.httpserver.HttpExchange
import com.sun.net.httpserver.HttpServer
import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.DefaultCiHttpTransport
import org.junit.jupiter.api.extension.AfterEachCallback
import org.junit.jupiter.api.extension.BeforeEachCallback
import org.junit.jupiter.api.extension.ExtensionContext
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.ServerSocket
import java.util.concurrent.CopyOnWriteArrayList

/** The token every transport of [ProviderApiServer] sends. */
internal const val PROVIDER_TOKEN = "provider-token"

/** The path under the server's origin where the provider API starts, as GitHub Enterprise's `/api/v3` does. */
internal const val API_PATH = "/api/v3"

/**
 * A request as the provider API received it. [target] is the raw request target, path and query, as sent; [headers]
 * holds only the headers the transport sets ([TRANSPORT_HEADERS]), an absent one as null, because the JDK client adds
 * its own (`Host`, `Content-Length`, the HTTP/2 upgrade).
 */
internal data class ReceivedRequest(
  val method: String,
  val target: String,
  val headers: Map<String, String?>,
  val body: String,
)

/** The headers the transport sets on a request. */
internal val TRANSPORT_HEADERS = listOf("Accept", "User-Agent", "Authorization", "Content-Type")

/**
 * A provider API on a local port. It answers every request with the [answer] a test gives it and records what it
 * received. Registered as an extension, it starts before each test and stops after it.
 */
internal class ProviderApiServer : BeforeEachCallback, AfterEachCallback {
  private lateinit var server: HttpServer
  private val requests = CopyOnWriteArrayList<ReceivedRequest>()

  @Volatile
  private var answer = Answer(200, ByteArray(0), emptyMap())

  /** The requests received, in order. */
  val received: List<ReceivedRequest> get() = requests.toList()

  /** Answers every request from now on with [status], the [body] text (none when empty) and the response [headers]. */
  fun answer(status: Int, body: String = "", headers: Map<String, String> = emptyMap()) {
    answer = Answer(status, body.toByteArray(Charsets.UTF_8), headers)
  }

  /** This server's origin, `http://127.0.0.1:<port>`. */
  val origin: String get() = "http://127.0.0.1:${server.address.port}"

  /** A transport of [provider] to this server's API, authenticated by [PROVIDER_TOKEN]. */
  fun transport(provider: CiProviderId = CiProviderId.GITHUB): DefaultCiHttpTransport =
    DefaultCiHttpTransport(provider, "$origin$API_PATH", PROVIDER_TOKEN)

  override fun beforeEach(context: ExtensionContext) {
    server = HttpServer.create(InetSocketAddress(InetAddress.getLoopbackAddress(), 0), 0)
    server.createContext("/", ::handle)
    server.start()
  }

  override fun afterEach(context: ExtensionContext) {
    // A test that interrupts its thread must not leave it interrupted for the next one, even when it fails.
    Thread.interrupted()
    server.stop(0)
  }

  private fun handle(exchange: HttpExchange) = exchange.use {
    requests += ReceivedRequest(
      exchange.requestMethod,
      exchange.requestURI.toString(),
      TRANSPORT_HEADERS.associateWith { exchange.requestHeaders.getFirst(it) },
      exchange.requestBody.readAllBytes().toString(Charsets.UTF_8),
    )
    val current = answer
    current.headers.forEach { (name, value) -> exchange.responseHeaders.add(name, value) }
    // -1 sends no body at all, as a 204 or a redirect does.
    exchange.sendResponseHeaders(current.status, if (current.body.isEmpty()) -1 else current.body.size.toLong())
    exchange.responseBody.write(current.body)
  }

  private class Answer(val status: Int, val body: ByteArray, val headers: Map<String, String>)
}

/** The largest provider response the transport reads: 16 MiB. */
internal const val MAX_RESPONSE_BYTES = 16 * 1024 * 1024

/** A JSON string literal whose UTF-8 text is exactly [bytes] long. */
internal fun jsonTextOfSize(bytes: Int): String = "\"${"x".repeat(bytes - 2)}\""

/** A transport of [provider] to a local port nothing listens on, so every connection is refused. */
internal fun unreachableTransport(provider: CiProviderId): DefaultCiHttpTransport {
  val port = ServerSocket(0, 1, InetAddress.getLoopbackAddress()).use { it.localPort }
  return DefaultCiHttpTransport(provider, "http://127.0.0.1:$port$API_PATH", PROVIDER_TOKEN)
}
