#!/usr/bin/env python3
"""
Validate that generated public Dockerfiles build on root-capable DHI base images.

scripts/dockerfiles.py generates each RC image's Dockerfile by flattening <lang>.Dockerfile with
its inlined <lang>-community bases. Docker resolves ARG-tagged FROM lines only from ARGs declared
before the first FROM (the "global" scope). Every `FROM dhi.io/...` base runs the image's
root-requiring build steps (apt-get install, chmod 777 $HOME, echo 'root:...' > /etc/passwd), so
each must resolve to a root DHI "-dev" variant; the hardened non-root variant breaks those steps.

This flattens each public variant with the real generator (substitute_from_directives). It fails
closed: a `FROM dhi.io/...` ref it cannot prove is "-dev" (a bare digest, an untagged base, an
unexpandable variable, an empty resolution) is a violation. Root-ness is the DHI "-dev" naming
convention (digest stripped) — an image's runtime user cannot be queried offline, and the lint CI
has no Docker daemon or dhi.io registry access.

The generator also appends a Jinja template snippet to each Dockerfile; the flatten does not render
it, so check_templates asserts the templates contain no FROM of their own.

Run from the repo root:
    ./scripts/validate_dockerfiles_base.py
"""
import glob
import re
import sys
from typing import List, Optional

from dockerfiles import load_variants, substitute_from_directives

BASE_DIR = "dockerfiles/base"

# Bare `FROM <name>` bases that are not build stages and never inlined includes.
KNOWN_BARE_BASES = {"scratch"}

# Docker treats FROM/ARG keywords case-insensitively and ignores leading whitespace before them;
# variable names stay case-sensitive.
FIRST_FROM_RE = re.compile(r"^[ \t]*FROM\s", re.MULTILINE | re.IGNORECASE)
DHI_FROM_RE = re.compile(r"^[ \t]*FROM\s+(?:--\S+\s+)*(dhi\.io/\S+)", re.MULTILINE | re.IGNORECASE)
REF_RE = re.compile(r"[:@](.+)$")
VAR_REF_RE = re.compile(r"^\$\{?(\w+)\}?$")
STAGE_NAME_RE = re.compile(r"^[ \t]*FROM\s+.+?\s+AS\s+(\S+)", re.MULTILINE | re.IGNORECASE)
BARE_FROM_RE = re.compile(r"^[ \t]*FROM\s+([A-Za-z][\w.-]*)\s*$", re.MULTILINE | re.IGNORECASE)
INDIRECT_FROM_RE = re.compile(r"^[ \t]*FROM\s+(?:--\S+\s+)*(\$\S+|\{\{.*?\}\})", re.MULTILINE | re.IGNORECASE)


def is_root_tag(tag: Optional[str]) -> bool:
    """DHI root build bases are the '-dev' variants; strip any @digest first."""
    return bool(tag) and tag.split("@", 1)[0].endswith("-dev")


def global_arg(flattened: str, name: str) -> Optional[str]:
    """The last global `ARG <name>` (before the first FROM) — the value Docker uses in FROM lines."""
    first_from = FIRST_FROM_RE.search(flattened)
    head = flattened[: first_from.start()] if first_from else flattened
    pat = re.compile(rf'^[ \t]*(?i:ARG)\s+{re.escape(name)}=(?:"([^"]*)"|(\S+))', re.MULTILINE)
    values = [quoted if quoted else bare for quoted, bare in pat.findall(head)]
    return values[-1] if values else None


def resolved_dhi_base_tags(flattened: str) -> List[str]:
    """Resolve every `dhi.io/<repo>` FROM to its tag; untagged/unexpandable-$VAR → "" (fail-closed)."""
    tags = []
    for image in DHI_FROM_RE.findall(flattened):
        ref = REF_RE.search(image[len("dhi.io/"):])
        if ref is None:
            tags.append("")  # untagged (implicit :latest) — unprovable
            continue
        token = ref.group(1)
        var = VAR_REF_RE.match(token)
        if var:
            tags.append(global_arg(flattened, var.group(1)) or "")
        elif "$" in token:
            tags.append("")  # unresolved composite variable (e.g. $BASE_TAG-dev) — fail-closed
        else:
            tags.append(token)
    return tags


def unresolved_includes(flattened: str) -> List[str]:
    """Bare `FROM <name>` left by a failed include (name is neither a build stage nor a known base)."""
    stages = {s.lower() for s in STAGE_NAME_RE.findall(flattened)}
    return sorted(
        {
            name
            for name in BARE_FROM_RE.findall(flattened)
            if name.lower() not in stages and name.lower() not in KNOWN_BARE_BASES
        }
    )


def check_flattened(variant: str, flattened: str) -> List[str]:
    msgs = [
        f"{variant}: unresolved FROM include (missing base file?): {name}"
        for name in unresolved_includes(flattened)
    ]

    msgs += [
        f"{variant}: FROM uses a variable image reference {ref!r} — cannot prove it is a root base"
        for ref in INDIRECT_FROM_RE.findall(flattened)
    ]

    tags = resolved_dhi_base_tags(flattened)
    if not tags:
        msgs.append(f"{variant}: no dhi.io base image FROM found (base check would pass vacuously)")
    msgs += [
        f"{variant}: FROM dhi.io base pins {tag!r}, not a provable root DHI '-dev' tag"
        for tag in tags
        if not is_root_tag(tag)
    ]
    return msgs


def check_templates(base_dir: str = BASE_DIR) -> List[str]:
    """The .j2 snippets are appended verbatim to each Dockerfile but never flattened here; they are
    trailing layers, so any FROM (literal or variable) would introduce an unvalidated build base."""
    templates = sorted(glob.glob(f"{base_dir}/templates/*.j2"))
    if not templates:
        return [f"no .j2 templates found under {base_dir}/templates — this guard would pass vacuously"]
    msgs = []
    for path in templates:
        with open(path, encoding="utf-8") as f:
            if FIRST_FROM_RE.search(f.read()):
                msgs.append(f"{path}: template must not contain a FROM (it would introduce an unvalidated base)")
    return msgs


def collect_violations(base_dir: str = BASE_DIR) -> List[str]:
    variants = load_variants()
    if not variants:
        return ["no variants found in public.json — nothing validated"]
    violations: List[str] = check_templates(base_dir)
    for variant, data in variants.items():
        base_source = data.get("from", variant)
        base_path = f"{base_dir}/{base_source}.Dockerfile"
        try:
            with open(base_path, encoding="utf-8") as f:
                content = f.read()
        except OSError as e:
            violations.append(f"{variant}: cannot read base {base_path}: {e}")
            continue
        violations.extend(check_flattened(variant, substitute_from_directives(content, base_dir)))
    return violations


def main() -> None:
    violations = collect_violations()
    if violations:
        print("ERROR: public Dockerfile base-image validation failed:")
        for v in violations:
            print(f"  - {v}")
        print(
            "\nEvery `FROM dhi.io/...` base runs the image's root build steps (apt-get, chmod, npm "
            "install -g) and must resolve to a root DHI '-dev' tag; the hardened variant runs non-root."
        )
        sys.exit(1)
    print("OK: all public variants build on root-capable dhi.io '-dev' base images.")


if __name__ == "__main__":
    main()
