#!/usr/bin/env python3
import argparse
import hashlib
import heapq
import json
from pathlib import Path

import numpy as np
from sentence_transformers import SentenceTransformer


def pair(left: int, right: int) -> tuple[int, int]:
    return (left, right) if left < right else (right, left)


def adjusted_distance(distance: float, left_size: int, right_size: int, alpha: float) -> float:
    return distance * (((left_size + right_size) / 2.0) ** alpha)


def cluster_embeddings(embeddings: np.ndarray, threshold: float, alpha: float) -> list[list[int]]:
    count = len(embeddings)
    if count == 0:
        return []

    similarities = embeddings @ embeddings.T
    distances: dict[tuple[int, int], float] = {}
    heap: list[tuple[float, int, int]] = []
    sizes = {index: 1 for index in range(count)}
    members = {index: [index] for index in range(count)}
    active = set(range(count))
    for left in range(count):
        for right in range(left + 1, count):
            distance = max(0.0, float(1.0 - similarities[left, right]))
            distances[(left, right)] = distance
            heapq.heappush(heap, (distance, left, right))

    next_id = count
    while heap:
        score, left, right = heapq.heappop(heap)
        if left not in active or right not in active:
            continue
        if score > threshold:
            break

        others = sorted(active - {left, right})
        active.remove(left)
        active.remove(right)
        merged = next_id
        next_id += 1
        left_size = sizes[left]
        right_size = sizes[right]
        sizes[merged] = left_size + right_size
        members[merged] = members[left] + members[right]
        active.add(merged)

        for other in others:
            left_distance = distances[pair(left, other)]
            right_distance = distances[pair(right, other)]
            distance = (left_size * left_distance + right_size * right_distance) / (left_size + right_size)
            distances[pair(merged, other)] = distance
            new_score = adjusted_distance(distance, sizes[merged], sizes[other], alpha)
            first, second = pair(merged, other)
            heapq.heappush(heap, (new_score, first, second))

    return [sorted(members[cluster_id]) for cluster_id in active]


def main() -> None:
    parser = argparse.ArgumentParser(description="Cluster Edict Next Signal embeddings")
    parser.add_argument("--manifest", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()

    manifest = json.loads(args.manifest.read_text(encoding="utf-8"))
    signals = manifest["signals"]
    args.output.mkdir(parents=True, exist_ok=True)
    if any(args.output.glob("*.json")):
        raise RuntimeError(f"Output directory already contains JSON files: {args.output}")
    if not signals:
        print(f"Wrote 0 clusters for 0 Signals to {args.output}")
        return

    model = SentenceTransformer(manifest["model"], revision=manifest["modelRevision"])
    embeddings = model.encode(
        [signal["description"] for signal in signals],
        normalize_embeddings=True,
        show_progress_bar=True,
    )
    embeddings = np.asarray(embeddings, dtype=np.float32)

    outputs = []
    for language in sorted({signal["language"] for signal in signals}):
        global_indices = [index for index, signal in enumerate(signals) if signal["language"] == language]
        language_embeddings = embeddings[global_indices]
        clusters = cluster_embeddings(
            language_embeddings,
            float(manifest["threshold"]),
            float(manifest["sizePenaltyAlpha"]),
        )
        for local_indices in clusters:
            signal_ids = sorted(signals[global_indices[index]]["id"] for index in local_indices)
            digest = hashlib.sha256("\n".join(signal_ids).encode("utf-8")).hexdigest()[:12]
            outputs.append({"id": f"stage1-{language.lower()}-{digest}", "language": language, "signalIds": signal_ids})

    outputs.sort(key=lambda item: (item["language"], item["signalIds"][0]))
    for index, output in enumerate(outputs, start=1):
        path = args.output / f"{index:04d}-{output['id']}.json"
        path.write_text(json.dumps(output, indent=2) + "\n", encoding="utf-8")
    print(f"Wrote {len(outputs)} clusters for {len(signals)} Signals to {args.output}")


if __name__ == "__main__":
    main()
