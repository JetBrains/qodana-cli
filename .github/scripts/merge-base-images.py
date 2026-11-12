"""Create multi-arch tags for one bake group of dockerfiles/base from per-platform digests.

Usage (from dockerfiles/base): merge-base-images.py GROUP DIGESTS_DIR

The base job pushes every target of GROUP untagged by digest, one platform per runner,
and saves the bake metadata as DIGESTS_DIR/GROUP-<arch>.json:
    {"<target>": {"image.name": "<repo>", "containerimage.digest": "sha256:..."}, ...}
Tags come from docker-bake.hcl (`bake --print` without PUSH_BY_DIGEST).
"""

import json
import os
import subprocess as sp
import sys
from pathlib import Path

ARCHES = ("amd64", "arm64")


def fail(message):
    print(f"::error::{message}")
    sys.exit(1)


def bake_tags(group):
    env = {k: v for k, v in os.environ.items() if k not in ("PUSH_BY_DIGEST", "BASE_PLATFORMS")}
    out = sp.run(["docker", "buildx", "bake", "--print", group], env=env, check=True, capture_output=True, text=True)
    targets = json.loads(out.stdout)["target"]
    return {name: target.get("tags") or [] for name, target in targets.items()}


def load_metadata(digests_dir, group):
    metadata = {}
    for arch in ARCHES:
        path = digests_dir / f"{group}-{arch}.json"
        if not path.is_file():
            fail(f"No {arch} build for group {group}")
        metadata[arch] = json.loads(path.read_text())
    return metadata


def main():
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    group, digests_dir = sys.argv[1], Path(sys.argv[2])

    metadata = load_metadata(digests_dir, group)
    # Validate the whole group before tagging anything, so its tags are never partially updated
    plan = []
    for target, tags in bake_tags(group).items():
        if not tags:
            fail(f"No tags for target {target} in docker-bake.hcl")
        sources = []
        for arch in ARCHES:
            entry = metadata[arch].get(target, {})
            name, digest = entry.get("image.name"), entry.get("containerimage.digest")
            if not name or not digest:
                fail(f"No {arch} digest for target {target}")
            sources.append(f"{name}@{digest}")
        plan.append((target, tags, sources))

    for target, tags, sources in plan:
        print(f"::group::{target} -> {' '.join(tags)}", flush=True)
        sp.run(["docker", "buildx", "imagetools", "create", *(f"--tag={t}" for t in tags), *sources], check=True)
        sp.run(["docker", "buildx", "imagetools", "inspect", tags[0]], check=True)
        print("::endgroup::", flush=True)


if __name__ == "__main__":
    main()
