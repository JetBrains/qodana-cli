import json
import os
import re
import subprocess as sp
import sys
from fnmatch import fnmatch
from pathlib import Path

PLATFORMS = (
    "linux/amd64",
    "linux/arm64"
)

EXCLUDE_PATTERNS = (
    "2023.2/*"
    "2023.3/cpp",
    "2023.3/android",
    "2024.1/android",
    "2023.3/cnova",
    "2024.1/cnova",
    "*/ruby",
    "2025.3/*",  # QD-12741
)

RUNNERS = {
    "linux/amd64": "ubuntu-24.04",
    "linux/arm64": "ubuntu-24.04-arm",
}

TARGET_BRANCH = os.getenv("GITHUB_BASE_REF")

def changed_in_this_pr(path):
    if TARGET_BRANCH is None:
        raise RuntimeError("this is not a pull request")
    head = os.getenv("GITHUB_SHA")

    try:
        sp.run(["git", "diff", "--no-patch", "--exit-code", f"origin/{TARGET_BRANCH}..{head}", "--", str(path)], check=True)
    except sp.CalledProcessError as exc:
        if exc.returncode == 1:
            return True  # if 1, true
        raise  # if not 0 or 1, raise an exception

    return False  # if 0, false

result = []
for version_dir in Path(".").glob("*"):
    if version_dir.name != "next" and not re.fullmatch(r"20\d\d\.\d", version_dir.name):
        continue  # Not a version folder

    for product_dir in version_dir.glob("*"):
        if product_dir.name == "base":
            continue  # Not a product

        if any(fnmatch(f"{version_dir.name}/{product_dir.name}", pattern) for pattern in EXCLUDE_PATTERNS):
            continue  # Excluded by EXCLUDE_PATTERNS

        if not (product_dir / "Dockerfile").exists():
            continue  # Dockerfile missing

        if TARGET_BRANCH is not None and not changed_in_this_pr(f"{version_dir.name}/{product_dir.name}/Dockerfile"):
            continue  # This is a PR and this release's Dockerfile was unchanged

        for platform in PLATFORMS:
            result.append({
                "version": version_dir.name,
                "linter": product_dir.name,
                "platform": platform,
                "runner": RUNNERS[platform],
            })

json.dump(result, sys.stdout, ensure_ascii=False)
json.dump(result, sys.stderr, ensure_ascii=False, indent="\t")