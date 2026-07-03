import os
import sys

import pytest

sys.path.insert(0, os.path.dirname(__file__))

import dockerfiles  # noqa: E402

REAL_DOWNLOADS = {
    "linux": {
        "Link": "https://example.test/qodana-linux.tar.gz",
        "ChecksumLink": "https://example.test/qodana-linux.tar.gz.sha256",
    },
    "linuxARM64": {
        "Link": "https://example.test/qodana-arm.tar.gz",
        "ChecksumLink": "https://example.test/qodana-arm.tar.gz.sha256",
    },
}


@pytest.fixture(autouse=True)
def _chdir_repo_root():
    old = os.getcwd()
    os.chdir(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
    yield
    os.chdir(old)


def _render_jvm(qd_version):
    env = dockerfiles.create_jinja_environment()
    intellij = dockerfiles.load_template(env, "dockerfiles/base/templates/intellij.Dockerfile.j2")
    thirdparty = dockerfiles.load_template(env, "dockerfiles/base/templates/thirdparty.Dockerfile.j2")
    data = dockerfiles.load_variants()["jvm"]
    return dockerfiles.generate_variant_dockerfile(
        "jvm", data, "dockerfiles/base", intellij, thirdparty, qd_version,
    )


def test_variant_without_matching_release_renders_empty_fallbacks(monkeypatch):
    monkeypatch.setattr(dockerfiles, "load_release_info", lambda *a, **k: {})
    out = _render_jvm("2026.2")
    assert out
    assert 'ARG QD_BUILD=""' in out
    assert 'ARG QD_DOWNLOAD_URL_LINUX=""' in out
    assert 'ARG QD_CHECKSUM_URL_LINUX=""' in out
    # Empty fallbacks are only safe because the template's non-release path fails loudly.
    assert 'No release information available for $QD_CODE $QD_VERSION" >&2 && exit 1' in out


def test_variant_with_release_info_renders_download_urls(monkeypatch):
    monkeypatch.setattr(dockerfiles, "load_release_info", lambda *a, **k: {
        "build": "262.9999", "downloads": REAL_DOWNLOADS,
    })
    out = _render_jvm("2026.2")
    assert 'ARG QD_BUILD="262.9999"' in out
    assert 'ARG QD_DOWNLOAD_URL_LINUX="https://example.test/qodana-linux.tar.gz"' in out
    assert 'ARG QD_CHECKSUM_URL_LINUX_ARM64="https://example.test/qodana-arm.tar.gz.sha256"' in out


def test_main_succeeds_and_writes_all_variants_for_empty_fallbacks(monkeypatch):
    monkeypatch.setattr(dockerfiles, "load_release_info", lambda *a, **k: {})
    written = {}
    monkeypatch.setattr(dockerfiles, "write_dockerfile",
                        lambda variant, content: written.__setitem__(variant, content))
    monkeypatch.setattr(sys, "argv", ["dockerfiles.py", "2026.2"])
    dockerfiles.main()
    assert set(written) == set(dockerfiles.load_variants())
    assert all(written.values())


def test_main_aggregates_all_variant_failures_and_exits(monkeypatch):
    # A failure outside OSError/ValueError is still caught and aggregated.
    attempted = []

    def boom(variant, *a, **k):
        attempted.append(variant)
        raise AttributeError("boom")

    monkeypatch.setattr(dockerfiles, "generate_variant_dockerfile", boom)
    monkeypatch.setattr(sys, "argv", ["dockerfiles.py", "2026.2"])
    with pytest.raises(SystemExit) as exc:
        dockerfiles.main()
    assert exc.value.code == 1
    assert set(attempted) == set(dockerfiles.load_variants())


def test_main_exits_nonzero_when_a_write_fails(monkeypatch):
    monkeypatch.setattr(dockerfiles, "load_release_info", lambda *a, **k: {})

    def boom(variant, content):
        raise OSError("disk full")

    monkeypatch.setattr(dockerfiles, "write_dockerfile", boom)
    monkeypatch.setattr(sys, "argv", ["dockerfiles.py", "2026.2"])
    with pytest.raises(SystemExit) as exc:
        dockerfiles.main()
    assert exc.value.code == 1


def test_main_exits_when_feed_downloads_are_null(monkeypatch):
    # A matched release with null Downloads makes the template render raise; the catch-all must
    # aggregate it and exit 1.
    monkeypatch.setattr(dockerfiles, "load_release_info",
                        lambda *a, **k: {"build": "262.1", "downloads": None})
    monkeypatch.setattr(dockerfiles, "write_dockerfile", lambda *a, **k: None)
    monkeypatch.setattr(sys, "argv", ["dockerfiles.py", "2026.2"])
    with pytest.raises(SystemExit) as exc:
        dockerfiles.main()
    assert exc.value.code == 1


def _fake_urlopen_from(fixture_path):
    data = open(fixture_path, "rb").read()

    class _Resp:
        def __enter__(self):
            return self

        def __exit__(self, *a):
            return False

        def read(self):
            return data

    return lambda url, timeout=10: _Resp()


def test_load_release_info_parses_real_feed_schema(monkeypatch):
    # Keep the template-consumed keys (linux/linuxARM64 -> Link/ChecksumLink) validated against
    # real feed shape.
    monkeypatch.setattr(dockerfiles.urllib.request, "urlopen",
                        _fake_urlopen_from("feed/qodana-jvm.releases.json"))
    info = dockerfiles.load_release_info("QDJVM", "2024.1")
    assert info["build"] == "241.20036"  # latest of the two 2024.1 releases (sorted by Type, Date)
    dl = info["downloads"]
    assert dl["linux"]["Link"] and dl["linux"]["ChecksumLink"]
    assert dl["linuxARM64"]["Link"] and dl["linuxARM64"]["ChecksumLink"]
    # A version not present in the feed yields {} (accepted pre-release case).
    assert dockerfiles.load_release_info("QDJVM", "1999.9") == {}


def test_load_release_info_raises_on_unknown_product_code():
    with pytest.raises(ValueError):
        dockerfiles.load_release_info("NOSUCHCODE", "2026.1")


def test_intellij_template_wires_overridable_qd_version_into_url(monkeypatch):
    monkeypatch.setattr(dockerfiles, "load_release_info", lambda *a, **k: {})
    out = _render_jvm("2026.2")
    assert 'ARG QD_VERSION="2026.2"' in out
    assert 'ENV QD_VERSION="$QD_VERSION"' in out
    assert "qodana/$QD_VERSION/" in out
