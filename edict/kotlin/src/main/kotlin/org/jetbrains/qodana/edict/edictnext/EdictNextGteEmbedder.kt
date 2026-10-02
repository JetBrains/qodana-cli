package org.jetbrains.qodana.edict.edictnext

import ai.djl.modality.nlp.DefaultVocabulary
import ai.djl.modality.nlp.bert.BertFullTokenizer
import ai.onnxruntime.OnnxTensor
import ai.onnxruntime.OrtEnvironment
import ai.onnxruntime.OrtSession
import java.io.OutputStream
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.nio.LongBuffer
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.StandardCopyOption
import java.security.DigestInputStream
import java.security.MessageDigest
import java.util.HexFormat
import kotlin.io.path.createDirectories
import kotlin.io.path.deleteIfExists
import kotlin.io.path.inputStream
import kotlin.io.path.isRegularFile
import kotlin.math.sqrt

/** Embeds texts like sentence-transformers `thenlper/gte-large`: BERT tokens, mean pooling, L2 normalization. */
internal class EdictNextGteEmbedder private constructor(
  private val session: OrtSession,
  private val vocabulary: DefaultVocabulary,
) : AutoCloseable {
  private val tokenizer = BertFullTokenizer(vocabulary, true)
  private val clsId = vocabulary.getIndex("[CLS]")
  private val sepId = vocabulary.getIndex("[SEP]")

  fun embed(texts: List<String>): List<FloatArray> {
    val tokenIds = texts.map { text ->
      buildList {
        add(clsId)
        tokenizer.tokenize(text).take(MAX_TOKENS - 2).mapTo(this, vocabulary::getIndex)
        add(sepId)
      }.toLongArray()
    }
    val length = tokenIds.maxOf(LongArray::size)
    val inputIds = LongArray(texts.size * length)
    val attentionMask = LongArray(inputIds.size)
    tokenIds.forEachIndexed { batch, ids ->
      ids.copyInto(inputIds, batch * length)
      attentionMask.fill(1, batch * length, batch * length + ids.size)
    }
    val shape = longArrayOf(texts.size.toLong(), length.toLong())
    val inputs = mapOf("input_ids" to inputIds, "attention_mask" to attentionMask, "token_type_ids" to LongArray(inputIds.size))
      .mapValues { OnnxTensor.createTensor(OrtEnvironment.getEnvironment(), LongBuffer.wrap(it.value), shape) }
    try {
      session.run(inputs).use { result ->
        val hidden = (result.get("last_hidden_state").orElseThrow() as OnnxTensor).floatBuffer
        return tokenIds.mapIndexed { batch, ids ->
          // The token sum equals the mean pooling up to a scale that normalization removes.
          val vector = FloatArray(DIMENSION)
          for (token in ids.indices) {
            val offset = (batch * length + token) * DIMENSION
            for (index in vector.indices) vector[index] += hidden.get(offset + index)
          }
          val norm = sqrt(vector.sumOf { (it * it).toDouble() }).toFloat()
          for (index in vector.indices) vector[index] /= norm
          vector
        }
      }
    }
    finally {
      inputs.values.forEach(OnnxTensor::close)
    }
  }

  override fun close() {
    session.close()
  }

  companion object {
    const val DIMENSION: Int = 1_024
    private const val MAX_TOKENS: Int = 512
    private const val MODEL_PATH: String = "onnx/model.onnx"
    private const val MODEL_SHA256: String = "8b74fc4b120c681d10f89231f66cadf8ac53cc30eba939c76254dc651bfbd12c"
    private const val VOCABULARY_PATH: String = "vocab.txt"
    private const val VOCABULARY_SHA256: String = "07eced375cec144d27c900241f3e339478dec958f92fddbc551f295c992038a3"

    /** Downloads missing or corrupted model files into [directory] before loading them. */
    fun open(directory: Path): EdictNextGteEmbedder {
      val vocabulary = DefaultVocabulary.builder()
        .optMinFrequency(1)
        .addFromTextFile(provide(directory, VOCABULARY_PATH, VOCABULARY_SHA256))
        .optUnknownToken("[UNK]")
        .build()
      val session = OrtEnvironment.getEnvironment().createSession(provide(directory, MODEL_PATH, MODEL_SHA256).toString())
      return EdictNextGteEmbedder(session, vocabulary)
    }

    private fun provide(directory: Path, relativePath: String, sha256: String): Path {
      val path = directory.resolve(relativePath)
      if (path.isRegularFile() && sha256(path) == sha256) {
        return path
      }
      path.parent.createDirectories()
      val part = path.resolveSibling("${path.fileName}.part")
      val uri = URI.create("https://huggingface.co/$EDICT_NEXT_MODEL/resolve/$EDICT_NEXT_MODEL_REVISION/$relativePath")
      val response = HttpClient.newBuilder().followRedirects(HttpClient.Redirect.NORMAL).build().use { client ->
        client.send(HttpRequest.newBuilder(uri).build(), HttpResponse.BodyHandlers.ofFile(part))
      }
      check(response.statusCode() == 200) { "Failed to download $uri: HTTP ${response.statusCode()}" }
      if (sha256(part) != sha256) {
        part.deleteIfExists()
        error("Downloaded $uri does not match SHA-256 $sha256")
      }
      return Files.move(part, path, StandardCopyOption.REPLACE_EXISTING, StandardCopyOption.ATOMIC_MOVE)
    }

    private fun sha256(path: Path): String {
      val digest = MessageDigest.getInstance("SHA-256")
      DigestInputStream(path.inputStream(), digest).use { it.transferTo(OutputStream.nullOutputStream()) }
      return HexFormat.of().formatHex(digest.digest())
    }
  }
}
