#!/usr/bin/env python3
"""Edict Next neighbour retrieval.

A pure function of its request file: prepare embeddings once, then find the nearest same-language Signals for the
selected inbox Signals.

The script never applies a distance threshold and never decides what a cluster is. It hands out
locality; membership is the agent's verdict.
"""
import argparse
import hashlib
import json
from pathlib import Path

import numpy as np
from sentence_transformers import SentenceTransformer

ENCODE_CHUNK = 32


def embedding_text(signal: dict) -> str:
    return signal["description"][:2500]


def embedding_hash(signal: dict) -> str:
    return hashlib.sha256(embedding_text(signal).encode("utf-8")).hexdigest()


def language(signal: dict) -> str:
    return Path(signal["fileRevision"]["path"]).suffix.lower()


def load_cached(cache_directory: Path, hashes: list[str]) -> dict[str, np.ndarray]:
    if not cache_directory.is_dir():
        return {}
    cached = {}
    for embedding_hash in set(hashes):
        path = cache_directory / f"{embedding_hash}.npy"
        if path.is_file():
            cached[embedding_hash] = np.load(path)
    return cached


def store_cached(cache_directory: Path, embeddings: dict[str, np.ndarray]) -> None:
    cache_directory.mkdir(parents=True, exist_ok=True)
    for embedding_hash, embedding in embeddings.items():
        np.save(cache_directory / f"{embedding_hash}.npy", embedding)


def embed(request: dict, entries: list[dict], cache_directory: Path) -> dict[str, np.ndarray]:
    """Embed every distinct Signal description not already cached. Returns hash -> unit vector."""
    hashes = [embedding_hash(entry) for entry in entries]
    resolved = load_cached(cache_directory, hashes)
    missing = {}
    for entry in entries:
        entry_hash = embedding_hash(entry)
        if entry_hash not in resolved:
            missing[entry_hash] = embedding_text(entry)
    if missing:
        model = SentenceTransformer(request["model"], revision=request["modelRevision"])
        missing_hashes = sorted(missing)
        # Cache each chunk as it finishes.
        for start in range(0, len(missing_hashes), ENCODE_CHUNK):
            chunk = missing_hashes[start:start + ENCODE_CHUNK]
            encoded = np.asarray(
                model.encode([missing[embedding_hash] for embedding_hash in chunk], normalize_embeddings=True),
                dtype=np.float32,
            )
            fresh = {embedding_hash: encoded[index] for index, embedding_hash in enumerate(chunk)}
            store_cached(cache_directory, fresh)
            resolved.update(fresh)
            print(f"Embedded {start + len(chunk)}/{len(missing_hashes)} descriptions", flush=True)
    return resolved


def by_language(corpus: list[dict]) -> dict[str, list[int]]:
    grouped: dict[str, list[int]] = {}
    for index, entry in enumerate(corpus):
        grouped.setdefault(language(entry), []).append(index)
    return grouped


def nearest(embeddings: np.ndarray, origin: int, candidates: list[int], count: int) -> list[tuple[int, float]]:
    """The `count` candidates closest to `origin`, nearest first. Cosine distance, ties broken by corpus order."""
    if not candidates:
        return []
    distances = 1.0 - embeddings[candidates] @ embeddings[origin]
    order = np.argsort(distances, kind="stable")[:count]
    return [(candidates[int(position)], float(distances[int(position)])) for position in order]


def run_neighbours(request: dict, cache_directory: Path) -> dict:
    corpus = request["corpus"]
    neighbour_count = int(request["neighbourCount"])
    corpus_index = {entry["id"]: index for index, entry in enumerate(corpus)}
    cached = embed(request, corpus, cache_directory)
    embeddings = np.asarray([cached[embedding_hash(entry)] for entry in corpus], dtype=np.float32)
    grouped = by_language(corpus)
    neighbours = []
    for signal_id in request["signalIds"]:
        index = corpus_index[signal_id]
        candidates = [other for other in grouped[language(corpus[index])] if other != index]
        closest = nearest(embeddings, index, candidates, neighbour_count)
        neighbours.append({
            "signalId": signal_id,
            "closest": [
                {
                    "signalId": corpus[other]["id"],
                    "distance": distance,
                }
                for other, distance in closest
            ],
        })
    return {"neighbours": neighbours}


def main() -> None:
    parser = argparse.ArgumentParser(description="Edict Next neighbour retrieval")
    parser.add_argument("--request", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--cache", type=Path, required=True)
    args = parser.parse_args()

    request = json.loads(args.request.read_text(encoding="utf-8"))
    response = run_neighbours(request, args.cache)

    args.output.write_text(json.dumps(response, indent=2) + "\n", encoding="utf-8")
    print(f"Wrote the neighbours response to {args.output}")


if __name__ == "__main__":
    main()
