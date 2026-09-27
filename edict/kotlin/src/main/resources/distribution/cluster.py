#!/usr/bin/env python3
"""Prepare same-language nearest Signal candidates without making clustering decisions."""
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
    for digest in set(hashes):
        path = cache_directory / f"{digest}.npy"
        if path.is_file():
            cached[digest] = np.load(path)
    return cached


def store_cached(cache_directory: Path, embeddings: dict[str, np.ndarray]) -> None:
    cache_directory.mkdir(parents=True, exist_ok=True)
    for digest, embedding in embeddings.items():
        np.save(cache_directory / f"{digest}.npy", embedding)


def embed(request: dict, entries: list[dict], cache_directory: Path) -> dict[str, np.ndarray]:
    hashes = [embedding_hash(entry) for entry in entries]
    resolved = load_cached(cache_directory, hashes)
    missing = {}
    for entry in entries:
        digest = embedding_hash(entry)
        if digest not in resolved:
            missing[digest] = embedding_text(entry)
    if missing:
        model = SentenceTransformer(request["model"], revision=request["modelRevision"])
        missing_hashes = sorted(missing)
        for start in range(0, len(missing_hashes), ENCODE_CHUNK):
            chunk = missing_hashes[start:start + ENCODE_CHUNK]
            encoded = np.asarray(
                model.encode([missing[digest] for digest in chunk], normalize_embeddings=True),
                dtype=np.float32,
            )
            fresh = {digest: encoded[index] for index, digest in enumerate(chunk)}
            store_cached(cache_directory, fresh)
            resolved.update(fresh)
            print(f"Embedded {start + len(chunk)}/{len(missing_hashes)} descriptions", flush=True)
    return resolved


def run_neighbours(request: dict, cache_directory: Path) -> dict:
    corpus = request["corpus"]
    count = int(request["neighbourCount"])
    corpus_index = {entry["id"]: index for index, entry in enumerate(corpus)}
    cached = embed(request, corpus, cache_directory)
    embeddings = np.asarray([cached[embedding_hash(entry)] for entry in corpus], dtype=np.float32)
    grouped = {}
    for index, entry in enumerate(corpus):
        grouped.setdefault(language(entry), []).append(index)
    neighbours = []
    for signal_id in request["signalIds"]:
        index = corpus_index[signal_id]
        candidates = [other for other in grouped[language(corpus[index])] if other != index]
        distances = 1.0 - embeddings[candidates] @ embeddings[index] if candidates else np.asarray([])
        order = np.argsort(distances, kind="stable")[:count]
        neighbours.append({
            "signalId": signal_id,
            "closest": [
                {"signalId": corpus[candidates[int(position)]]["id"], "distance": float(distances[int(position)])}
                for position in order
            ],
        })
    return {"neighbours": neighbours}


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--request", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--cache", type=Path, required=True)
    args = parser.parse_args()
    request = json.loads(args.request.read_text(encoding="utf-8"))
    args.output.write_text(json.dumps(run_neighbours(request, args.cache), indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
