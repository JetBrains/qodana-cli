import os
import sys

import pytest

sys.path.insert(0, os.path.dirname(__file__))

from dockerfiles import load_variants, substitute_from_directives  # noqa: E402
from validate_dockerfiles_base import (  # noqa: E402
    check_flattened,
    check_templates,
    collect_violations,
    global_arg,
    is_root_tag,
    resolved_dhi_base_tags,
    unresolved_includes,
)

# Flattened shapes as substitute_from_directives emits them.
SHADOWED = '''ARG BASE_TAG="trixie"
ARG NODE_TAG="22-debian13-dev@sha256:abc"
FROM dhi.io/node:$NODE_TAG AS node_base
ARG BASE_TAG="trixie-debian13-dev"
FROM dhi.io/debian-base:$BASE_TAG
'''
FIXED = SHADOWED.replace('ARG BASE_TAG="trixie"', 'ARG BASE_TAG="trixie-debian13-dev"')
COMMUNITY = 'ARG BASE_TAG="trixie-debian13-dev"\nFROM dhi.io/debian-base:$BASE_TAG\n'
LANG_BASE = 'ARG GO_TAG="1.26-debian13-dev"\nFROM dhi.io/golang:$GO_TAG\n'
LANG_BASE_NONROOT = 'ARG GO_TAG="1.26"\nFROM dhi.io/golang:$GO_TAG\n'
BRACE = 'ARG BASE_TAG="trixie-debian13-dev"\nFROM dhi.io/debian-base:${BASE_TAG}\n'
UNQUOTED_ROOT = 'ARG BASE_TAG=trixie-debian13-dev\nFROM dhi.io/debian-base:$BASE_TAG\n'
LAST_WINS = ('ARG BASE_TAG="trixie"\nARG BASE_TAG="trixie-debian13-dev"\n'
             'FROM dhi.io/debian-base:$BASE_TAG\n')
PLATFORM_LITERAL = 'FROM --platform=$TARGETPLATFORM dhi.io/debian-base:trixie\n'
DIGEST_ONLY = 'FROM dhi.io/debian-base@sha256:abc123\n'
NON_DHI = 'FROM debian:bookworm-slim\nRUN echo hi\n'
UNTAGGED = 'ARG BASE_TAG="trixie-debian13-dev"\nFROM dhi.io/debian-base:$BASE_TAG\nFROM dhi.io/newbase\n'
UNDEFINED_VAR = 'ARG BASE_TAG="trixie-debian13-dev"\nFROM dhi.io/debian-base:$OTHER_TAG\n'
COMPOSITE_VAR = 'ARG BASE_TAG="trixie"\nFROM dhi.io/debian-base:$BASE_TAG-dev\n'
LOWER_FROM = 'ARG BASE_TAG="trixie"\nfrom dhi.io/debian-base:$BASE_TAG\n'
INDENTED_FROM = 'ARG BASE_TAG="trixie"\n    FROM dhi.io/debian-base:$BASE_TAG\n'
INDIRECT_IMAGE = 'ARG BASE_TAG="trixie-debian13-dev"\nFROM dhi.io/debian-base:$BASE_TAG\nFROM $SOME_IMAGE\n'


@pytest.fixture(autouse=True)
def _chdir_repo_root():
    old = os.getcwd()
    os.chdir(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
    yield
    os.chdir(old)


def test_is_root_tag():
    assert is_root_tag("trixie-debian13-dev")
    assert is_root_tag("trixie-debian13-dev@sha256:abc")
    assert is_root_tag("8.4-dev")
    assert not is_root_tag("trixie")
    assert not is_root_tag("sha256:abc")
    assert not is_root_tag(None)
    assert not is_root_tag("")


def test_global_arg_scope_last_wins_and_unquoted():
    assert global_arg(SHADOWED, "BASE_TAG") == "trixie"  # community ARG (after a FROM) is ignored
    assert global_arg(SHADOWED, "NODE_TAG") == "22-debian13-dev@sha256:abc"
    assert global_arg(COMMUNITY, "BASE_TAG") == "trixie-debian13-dev"
    assert global_arg(UNQUOTED_ROOT, "BASE_TAG") == "trixie-debian13-dev"
    assert global_arg(LAST_WINS, "BASE_TAG") == "trixie-debian13-dev"
    assert global_arg(COMMUNITY, "MISSING") is None


def test_resolved_tags_cover_every_dhi_base_and_form():
    assert resolved_dhi_base_tags(SHADOWED) == ["22-debian13-dev@sha256:abc", "trixie"]
    assert resolved_dhi_base_tags(LANG_BASE) == ["1.26-debian13-dev"]
    assert resolved_dhi_base_tags(BRACE) == ["trixie-debian13-dev"]
    assert resolved_dhi_base_tags(PLATFORM_LITERAL) == ["trixie"]
    assert resolved_dhi_base_tags(DIGEST_ONLY) == ["sha256:abc123"]
    assert resolved_dhi_base_tags(NON_DHI) == []


def test_check_flattened_flags_non_root_across_bases():
    shadow = check_flattened("jvm", SHADOWED)
    assert any("'trixie'" in m and "not a provable root" in m for m in shadow)
    assert check_flattened("jvm", FIXED) == []
    assert check_flattened("jvm-community", COMMUNITY) == []
    assert check_flattened("go", LANG_BASE) == []
    go_bad = check_flattened("go", LANG_BASE_NONROOT)  # non-debian DHI base is guarded too
    assert len(go_bad) == 1 and "'1.26'" in go_bad[0]
    assert len(check_flattened("d", DIGEST_ONLY)) == 1  # fail-closed on a bare digest


def test_check_flattened_flags_variant_with_no_dhi_base():
    out = check_flattened("weird", NON_DHI)
    assert len(out) == 1 and "no dhi.io base image FROM" in out[0]


def test_untagged_dhi_base_fails_closed():
    # an untagged `FROM dhi.io/newbase` is an implicit :latest — unprovable
    assert resolved_dhi_base_tags(UNTAGGED) == ["trixie-debian13-dev", ""]
    assert len(check_flattened("x", UNTAGGED)) == 1


def test_variable_image_reference_fails_closed():
    out = check_flattened("x", INDIRECT_IMAGE)
    assert len(out) == 1 and "variable image reference" in out[0]


def test_undefined_variable_fails_closed():
    assert resolved_dhi_base_tags(UNDEFINED_VAR) == [""]
    out = check_flattened("x", UNDEFINED_VAR)
    assert len(out) == 1 and "not a provable root" in out[0]


def test_composite_variable_ref_fails_closed():
    # `$BASE_TAG-dev` is not a pure variable, so it is unresolvable, not a literal ending in -dev
    assert resolved_dhi_base_tags(COMPOSITE_VAR) == [""]
    assert len(check_flattened("x", COMPOSITE_VAR)) == 1


def test_real_templates_introduce_no_dhi_base():
    # passes only because real templates exist and are clean; an empty match is a loud violation
    assert check_templates() == []


def test_template_with_any_from_is_flagged(tmp_path):
    # a variable-image FROM must be caught too, not only a literal dhi.io one
    templates = tmp_path / "templates"
    templates.mkdir()
    (templates / "bad.Dockerfile.j2").write_text("FROM {{ base }}\nRUN echo hi\n")
    out = check_templates(str(tmp_path))
    assert len(out) == 1 and "must not contain a FROM" in out[0]


def test_no_templates_is_loud(tmp_path):
    (tmp_path / "templates").mkdir()
    out = check_templates(str(tmp_path))
    assert len(out) == 1 and "pass vacuously" in out[0]


def test_lowercase_from_is_validated():
    assert resolved_dhi_base_tags(LOWER_FROM) == ["trixie"]
    assert any("not a provable root" in m for m in check_flattened("x", LOWER_FROM))


def test_indented_from_is_validated():
    assert resolved_dhi_base_tags(INDENTED_FROM) == ["trixie"]
    assert any("not a provable root" in m for m in check_flattened("x", INDENTED_FROM))


def test_empty_public_json_is_loud(monkeypatch):
    import validate_dockerfiles_base as v
    monkeypatch.setattr(v, "load_variants", lambda *a, **k: {})
    out = v.collect_violations()
    assert out and "no variants found" in out[0]


def test_unresolved_include_detected_but_stages_and_scratch_ignored():
    flattened = substitute_from_directives("FROM does-not-exist-base\n", "dockerfiles/base")
    assert unresolved_includes(flattened) == ["does-not-exist-base"]
    staged = "FROM dhi.io/debian-base:trixie-debian13-dev AS builder\nFROM builder\n"
    assert unresolved_includes(staged) == []
    assert unresolved_includes("FROM scratch\n") == []


def test_check_flattened_reports_unresolved_and_still_base_checks():
    both = "FROM ghost-base\nFROM dhi.io/debian-base:trixie\n"
    out = check_flattened("x", both)
    assert any("unresolved FROM include" in m and "ghost-base" in m for m in out)
    assert any("not a provable root" in m for m in out)


def test_missing_base_dir_is_loud():
    violations = collect_violations(base_dir="/definitely/not/here")
    assert any("cannot read base" in v for v in violations)


def test_real_variants_resolve_to_root_dev_tags():
    for variant, data in load_variants().items():
        base = data.get("from", variant)
        with open(f"dockerfiles/base/{base}.Dockerfile", encoding="utf-8") as fh:
            flattened = substitute_from_directives(fh.read(), "dockerfiles/base")
        tags = resolved_dhi_base_tags(flattened)
        assert tags, f"{variant}: no dhi.io base FROM found"
        for tag in tags:
            assert is_root_tag(tag), f"{variant}: dhi.io base resolves to {tag!r}"


def test_real_base_files_pass():
    assert collect_violations() == []
