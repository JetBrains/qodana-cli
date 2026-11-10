package org.jetbrains.qodana.edict.core

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withContext
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.nio.file.Files
import java.nio.file.Path
import java.security.MessageDigest
import java.util.HexFormat

/** Finds the nearest same-language Signals for selected inbox Signals with cached GTE embeddings. */
internal class EdictNeighbourFinder(private val modelDirectory: Path) {
  suspend fun find(state: EdictRepositoryState, signalIds: List<String>): Map<String, EdictSignalNeighbours> {
    if (signalIds.isEmpty()) {
      return emptyMap()
    }
    val corpus = state.clusters.flatMap(EdictStoredCluster::signals) + signalIds.map(state.signalsById::getValue)
    val texts = corpus.map { it.description.take(MAX_DESCRIPTION_LENGTH) }
    val hashes = texts.map(::sha256)
    val textsByHash = hashes.zip(texts).toMap()
    val cache = GteEmbeddingCache(EdictRepositoryDirectory(state.root).gteEmbeddingCacheDirectory)
    val vectorsByHash = withContext(Dispatchers.IO) {
      textsByHash.keys.mapNotNull { hash -> cache.read(hash)?.let { hash to it } }.toMap(HashMap())
    }
    val missing = textsByHash.filterKeys { it !in vectorsByHash }.entries.sortedBy { it.value.length }
    if (missing.isNotEmpty()) {
      withContext(Dispatchers.IO) { EdictGteEmbedder.open(modelDirectory) }.use { embedder ->
        withContext(Dispatchers.Default) {
          for (chunk in missing.chunked(EMBED_BATCH_SIZE)) {
            ensureActive()
            embedder.embed(chunk.map { it.value }).forEachIndexed { index, vector ->
              cache.write(chunk[index].key, vector)
              vectorsByHash[chunk[index].key] = vector
            }
          }
        }
      }
    }

    val vectors = hashes.map(vectorsByHash::getValue)
    val indicesById = corpus.withIndex().associate { (index, signal) -> signal.id to index }
    val indicesByLanguage = corpus.indices.groupBy { language(corpus[it]) }
    return signalIds.associateWith { signalId ->
      val origin = indicesById.getValue(signalId)
      val closest = indicesByLanguage.getValue(language(corpus[origin])).asSequence()
        .filter { it != origin }
        .map { index -> index to cosineDistance(vectors[origin], vectors[index]) }
        .sortedWith(compareBy<Pair<Int, Double>> { it.second }.thenBy { it.first })
        .take(EDICT_NEIGHBOUR_COUNT)
        .map { (index, distance) -> EdictNeighbour(corpus[index].id, distance) }
        .toList()
      EdictSignalNeighbours(signalId, closest)
    }
  }

  private fun language(signal: EdictSignal): String = signal.fileRevision.path.substringAfterLast('.', "").lowercase()

  private fun sha256(text: String): String =
    HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256").digest(text.toByteArray(Charsets.UTF_8)))

  private fun cosineDistance(left: FloatArray, right: FloatArray): Double {
    var similarity = 0.0
    for (index in left.indices) similarity += left[index] * right[index]
    return 1.0 - similarity
  }

  private companion object {
    const val MAX_DESCRIPTION_LENGTH: Int = 2_500
    const val EMBED_BATCH_SIZE: Int = 8
  }
}

private class GteEmbeddingCache(private val directory: Path) {
  fun read(hash: String): FloatArray? {
    val path = directory.resolve("$hash.f32")
    if (!Files.isRegularFile(path)) {
      return null
    }
    val vector = FloatArray(EdictGteEmbedder.DIMENSION)
    ByteBuffer.wrap(Files.readAllBytes(path)).order(ByteOrder.LITTLE_ENDIAN).asFloatBuffer().get(vector)
    return vector
  }

  fun write(hash: String, vector: FloatArray) {
    Files.createDirectories(directory)
    val bytes = ByteBuffer.allocate(vector.size * Float.SIZE_BYTES).order(ByteOrder.LITTLE_ENDIAN)
    vector.forEach(bytes::putFloat)
    Files.write(directory.resolve("$hash.f32"), bytes.array())
  }
}
