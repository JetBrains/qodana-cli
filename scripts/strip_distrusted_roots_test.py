"""Tests of strip-distrusted-roots.py, which dockerfiles/base/qodana-tools.Dockerfile writes into the images."""
import base64
import hashlib
import io
import os
import re
import subprocess
import sys
import zipfile

import pytest

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
TOOLS = os.path.join(REPO, "dockerfiles", "base", "qodana-tools.Dockerfile")
BUNDLE = "pip/_vendor/certifi/cacert.pem"

# Public root certificates, as certifi has them: a trusted one and the two distrusted ones (QD-16607)
TRUSTED = b"""\
# Issuer: CN=ISRG Root X1 O=Internet Security Research Group
# Subject: CN=ISRG Root X1 O=Internet Security Research Group
# Label: "ISRG Root X1"
# Serial: 172886928669790476064670243504169061120
# MD5 Fingerprint: 0c:d2:f9:e0:da:17:73:e9:ed:86:4d:a5:e3:70:e7:4e
# SHA1 Fingerprint: ca:bd:2a:79:a1:07:6a:31:f2:1d:25:36:35:cb:03:9d:43:29:a5:e8
# SHA256 Fingerprint: 96:bc:ec:06:26:49:76:f3:74:60:77:9a:cf:28:c5:a7:cf:e8:a3:c0:aa:e1:1a:8f:fc:ee:05:c0:bd:df:08:c6
-----BEGIN CERTIFICATE-----
MIIFazCCA1OgAwIBAgIRAIIQz7DSQONZRGPgu2OCiwAwDQYJKoZIhvcNAQELBQAw
TzELMAkGA1UEBhMCVVMxKTAnBgNVBAoTIEludGVybmV0IFNlY3VyaXR5IFJlc2Vh
cmNoIEdyb3VwMRUwEwYDVQQDEwxJU1JHIFJvb3QgWDEwHhcNMTUwNjA0MTEwNDM4
WhcNMzUwNjA0MTEwNDM4WjBPMQswCQYDVQQGEwJVUzEpMCcGA1UEChMgSW50ZXJu
ZXQgU2VjdXJpdHkgUmVzZWFyY2ggR3JvdXAxFTATBgNVBAMTDElTUkcgUm9vdCBY
MTCCAiIwDQYJKoZIhvcNAQEBBQADggIPADCCAgoCggIBAK3oJHP0FDfzm54rVygc
h77ct984kIxuPOZXoHj3dcKi/vVqbvYATyjb3miGbESTtrFj/RQSa78f0uoxmyF+
0TM8ukj13Xnfs7j/EvEhmkvBioZxaUpmZmyPfjxwv60pIgbz5MDmgK7iS4+3mX6U
A5/TR5d8mUgjU+g4rk8Kb4Mu0UlXjIB0ttov0DiNewNwIRt18jA8+o+u3dpjq+sW
T8KOEUt+zwvo/7V3LvSye0rgTBIlDHCNAymg4VMk7BPZ7hm/ELNKjD+Jo2FR3qyH
B5T0Y3HsLuJvW5iB4YlcNHlsdu87kGJ55tukmi8mxdAQ4Q7e2RCOFvu396j3x+UC
B5iPNgiV5+I3lg02dZ77DnKxHZu8A/lJBdiB3QW0KtZB6awBdpUKD9jf1b0SHzUv
KBds0pjBqAlkd25HN7rOrFleaJ1/ctaJxQZBKT5ZPt0m9STJEadao0xAH0ahmbWn
OlFuhjuefXKnEgV4We0+UXgVCwOPjdAvBbI+e0ocS3MFEvzG6uBQE3xDk3SzynTn
jh8BCNAw1FtxNrQHusEwMFxIt4I7mKZ9YIqioymCzLq9gwQbooMDQaHWBfEbwrbw
qHyGO0aoSCqI3Haadr8faqU9GY/rOPNk3sgrDQoo//fb4hVC1CLQJ13hef4Y53CI
rU7m2Ys6xt0nUW7/vGT1M0NPAgMBAAGjQjBAMA4GA1UdDwEB/wQEAwIBBjAPBgNV
HRMBAf8EBTADAQH/MB0GA1UdDgQWBBR5tFnme7bl5AFzgAiIyBpY9umbbjANBgkq
hkiG9w0BAQsFAAOCAgEAVR9YqbyyqFDQDLHYGmkgJykIrGF1XIpu+ILlaS/V9lZL
ubhzEFnTIZd+50xx+7LSYK05qAvqFyFWhfFQDlnrzuBZ6brJFe+GnY+EgPbk6ZGQ
3BebYhtF8GaV0nxvwuo77x/Py9auJ/GpsMiu/X1+mvoiBOv/2X/qkSsisRcOj/KK
NFtY2PwByVS5uCbMiogziUwthDyC3+6WVwW6LLv3xLfHTjuCvjHIInNzktHCgKQ5
ORAzI4JMPJ+GslWYHb4phowim57iaztXOoJwTdwJx4nLCgdNbOhdjsnvzqvHu7Ur
TkXWStAmzOVyyghqpZXjFaH3pO3JLF+l+/+sKAIuvtd7u+Nxe5AW0wdeRlN8NwdC
jNPElpzVmbUq4JUagEiuTDkHzsxHpFKVK7q4+63SM1N95R1NbdWhscdCb+ZAJzVc
oyi3B43njTOQ5yOf+1CceWxG1bQVs5ZufpsMljq4Ui0/1lvh+wjChP4kqKOJ2qxq
4RgqsahDYVvTH9w7jXbyLeiNdd8XM2w9U/t7y0Ff/9yi0GE44Za4rF2LN9d11TPA
mRGunUHBcnWEvgJBQl9nJEiU0Zsnvgc/ubhPgXRR4Xq37Z0j4r7g1SgEEzwxA57d
emyPxgcYxn/eR44/KJ4EBs+lVDR3veyJm+kXQ99b21/+jh5Xos1AnX5iItreGCc=
-----END CERTIFICATE-----
"""

EMSIGN_ECC_G3 = b"""\
# Issuer: CN=emSign ECC Root CA - G3 O=eMudhra Technologies Limited OU=emSign PKI
# Subject: CN=emSign ECC Root CA - G3 O=eMudhra Technologies Limited OU=emSign PKI
# Label: "emSign ECC Root CA - G3"
# Serial: 287880440101571086945156
# MD5 Fingerprint: ce:0b:72:d1:9f:88:8e:d0:50:03:e8:e3:b8:8b:67:40
# SHA1 Fingerprint: 30:43:fa:4f:f2:57:dc:a0:c3:80:ee:2e:58:ea:78:b2:3f:e6:bb:c1
# SHA256 Fingerprint: 86:a1:ec:ba:08:9c:4a:8d:3b:be:27:34:c6:12:ba:34:1d:81:3e:04:3c:f9:e8:a8:62:cd:5c:57:a3:6b:be:6b
-----BEGIN CERTIFICATE-----
MIICTjCCAdOgAwIBAgIKPPYHqWhwDtqLhDAKBggqhkjOPQQDAzBrMQswCQYDVQQG
EwJJTjETMBEGA1UECxMKZW1TaWduIFBLSTElMCMGA1UEChMcZU11ZGhyYSBUZWNo
bm9sb2dpZXMgTGltaXRlZDEgMB4GA1UEAxMXZW1TaWduIEVDQyBSb290IENBIC0g
RzMwHhcNMTgwMjE4MTgzMDAwWhcNNDMwMjE4MTgzMDAwWjBrMQswCQYDVQQGEwJJ
TjETMBEGA1UECxMKZW1TaWduIFBLSTElMCMGA1UEChMcZU11ZGhyYSBUZWNobm9s
b2dpZXMgTGltaXRlZDEgMB4GA1UEAxMXZW1TaWduIEVDQyBSb290IENBIC0gRzMw
djAQBgcqhkjOPQIBBgUrgQQAIgNiAAQjpQy4LRL1KPOxst3iAhKAnjlfSU2fySU0
WXTsuwYc58Byr+iuL+FBVIcUqEqy6HyC5ltqtdyzdc6LBtCGI79G1Y4PPwT01xyS
fvalY8L1X44uT6EYGQIrMgqCZH0Wk9GjQjBAMB0GA1UdDgQWBBR8XQKEE9TMipuB
zhccLikenEhjQjAOBgNVHQ8BAf8EBAMCAQYwDwYDVR0TAQH/BAUwAwEB/zAKBggq
hkjOPQQDAwNpADBmAjEAvvNhzwIQHWSVB7gYboiFBS+DCBeQyh+KTOgNG3qxrdWB
CUfvO6wIBHxcmbHtRwfSAjEAnbpV/KlK6O3t5nYBQnvI+GDZjVGLVTv7jHvrZQnD
+JbNR6iC8hZVdyR+EhCVBCyj
-----END CERTIFICATE-----
"""

AAA_CERTIFICATE_SERVICES = b"""\
# Issuer: CN=AAA Certificate Services O=Comodo CA Limited
# Subject: CN=AAA Certificate Services O=Comodo CA Limited
# Label: "Comodo AAA Services root"
# Serial: 1
# MD5 Fingerprint: 49:79:04:b0:eb:87:19:ac:47:b0:bc:11:51:9b:74:d0
# SHA1 Fingerprint: d1:eb:23:a4:6d:17:d6:8f:d9:25:64:c2:f1:f1:60:17:64:d8:e3:49
# SHA256 Fingerprint: d7:a7:a0:fb:5d:7e:27:31:d7:71:e9:48:4e:bc:de:f7:1d:5f:0c:3e:0a:29:48:78:2b:c8:3e:e0:ea:69:9e:f4
-----BEGIN CERTIFICATE-----
MIIEMjCCAxqgAwIBAgIBATANBgkqhkiG9w0BAQUFADB7MQswCQYDVQQGEwJHQjEb
MBkGA1UECAwSR3JlYXRlciBNYW5jaGVzdGVyMRAwDgYDVQQHDAdTYWxmb3JkMRow
GAYDVQQKDBFDb21vZG8gQ0EgTGltaXRlZDEhMB8GA1UEAwwYQUFBIENlcnRpZmlj
YXRlIFNlcnZpY2VzMB4XDTA0MDEwMTAwMDAwMFoXDTI4MTIzMTIzNTk1OVowezEL
MAkGA1UEBhMCR0IxGzAZBgNVBAgMEkdyZWF0ZXIgTWFuY2hlc3RlcjEQMA4GA1UE
BwwHU2FsZm9yZDEaMBgGA1UECgwRQ29tb2RvIENBIExpbWl0ZWQxITAfBgNVBAMM
GEFBQSBDZXJ0aWZpY2F0ZSBTZXJ2aWNlczCCASIwDQYJKoZIhvcNAQEBBQADggEP
ADCCAQoCggEBAL5AnfRu4ep2hxxNRUSOvkbIgwadwSr+GB+O5AL686tdUIoWMQua
BtDFcCLNSS1UY8y2bmhGC1Pqy0wkwLxyTurxFa70VJoSCsN6sjNg4tqJVfMiWPPe
3M/vg4aijJRPn2jymJBGhCfHdr/jzDUsi14HZGWCwEiwqJH5YZ92IFCokcdmtet4
YgNW8IoaE+oxox6gmf049vYnMlhvB/VruPsUK6+3qszWY19zjNoFmag4qMsXeDZR
rOme9Hg6jc8P2ULimAyrL58OAd7vn5lJ8S3frHRNG5i1R8XlKdH5kBjHYpy+g8cm
ez6KJcfA3Z3mNWgQIJ2P2N7Sw4ScDV7oL8kCAwEAAaOBwDCBvTAdBgNVHQ4EFgQU
oBEKIz6W8Qfs4q8p74Klf9AwpLQwDgYDVR0PAQH/BAQDAgEGMA8GA1UdEwEB/wQF
MAMBAf8wewYDVR0fBHQwcjA4oDagNIYyaHR0cDovL2NybC5jb21vZG9jYS5jb20v
QUFBQ2VydGlmaWNhdGVTZXJ2aWNlcy5jcmwwNqA0oDKGMGh0dHA6Ly9jcmwuY29t
b2RvLm5ldC9BQUFDZXJ0aWZpY2F0ZVNlcnZpY2VzLmNybDANBgkqhkiG9w0BAQUF
AAOCAQEACFb8AvCb6P+k+tZ7xkSAzk/ExfYAWMymtrwUSWgEdujm7l3sAg9g1o1Q
GE8mTgHj5rCl7r+8dFRBv/38ErjHT1r0iWAFf2C3BUrz9vHCv8S5dIa2LX1rzNLz
Rt0vxuBqw8M0Ayx9lt1awg6nCpnBBYurDC/zXDrPbDdVCYfeU0BsWO/8tqtlbgT2
G9w84FoVxp7Z8VlIMCFlA2zs6SFz7JsDoeA3raAVGI/6ugLOpyypEBMs1OUIJqsi
l2D4kF501KKaU73yqWjgom7C12yxow+ev+to51byrvLjKzg6CYG1a4XXvi3tPxq3
smPi9WIsgtRqAEFQ8TmDn5XpNpaYbg==
-----END CERTIFICATE-----
"""



@pytest.fixture(scope="module")
def tool(tmp_path_factory):
    source = open(TOOLS).read()
    script = re.search(r'^COPY <<"EOF" /strip-distrusted-roots\.py\n(.*?)^EOF$', source, re.S | re.M).group(1)
    path = tmp_path_factory.mktemp("tool") / "strip-distrusted-roots.py"
    path.write_text(script)
    return path


def run(tool, *paths):
    return subprocess.run([sys.executable, "-I", str(tool), *map(str, paths)], capture_output=True, text=True)


def record_hash(data):
    return "sha256=" + base64.urlsafe_b64encode(hashlib.sha256(data).digest()).rstrip(b"=").decode()


def wheel(bundle=TRUSTED + b"\n" + EMSIGN_ECC_G3 + b"\n" + AAA_CERTIFICATE_SERVICES):
    out = io.BytesIO()
    with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as z:
        z.writestr("pip/__init__.py", b"")
        z.writestr(BUNDLE, bundle)
        z.writestr("pip-1.0.dist-info/RECORD", f"pip/__init__.py,{record_hash(b'')},0\n"
                                                f"{BUNDLE},{record_hash(bundle)},{len(bundle)}\n"
                                                "pip-1.0.dist-info/RECORD,,\n")
    return out.getvalue()


def archive(entries, prefix=b""):
    out = io.BytesIO(prefix)
    out.seek(len(prefix))
    with zipfile.ZipFile(out, "w") as z:
        for name, data in entries.items():
            z.writestr(name, data)
    return out.getvalue()


def assert_clean(data):
    with zipfile.ZipFile(io.BytesIO(data)) as z:
        bundle = z.read(BUNDLE)
        assert bundle == TRUSTED + b"\n"
        for line in z.read("pip-1.0.dist-info/RECORD").decode().splitlines():
            path, digest, size = line.rsplit(",", 2)
            if digest:
                assert digest == record_hash(z.read(path)) and int(size) == len(z.read(path)), path


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def test_strips_a_wheel_and_updates_its_record(tool, tmp_path):
    path = tmp_path / "pip-1.0-py3-none-any.whl"
    path.write_bytes(wheel())
    result = run(tool, path)
    assert result.returncode == 0, result.stderr
    assert "removed 2 of 3 certificates" in result.stdout
    assert_clean(path.read_bytes())


def test_leaves_clean_files_byte_identical(tool, tmp_path):
    clean = tmp_path / "pip-1.0-py3-none-any.whl"
    clean.write_bytes(wheel(TRUSTED))
    pem = tmp_path / "roots.pem"
    pem.write_bytes(TRUSTED)
    before = clean.read_bytes(), pem.read_bytes()
    assert run(tool, clean, pem).returncode == 0
    assert (clean.read_bytes(), pem.read_bytes()) == before


def test_updates_the_pin_of_a_wheel_on_disk_and_drops_its_bytecode(tool, tmp_path):
    original = wheel()
    path = tmp_path / "pip-1.0-py3-none-any.whl"
    path.write_bytes(original)
    (tmp_path / "__init__.py").write_text(f'BUNDLE_SHA256 = {{"{path.name}": "{sha256(original)}"}}\n')
    (tmp_path / "__pycache__").mkdir()
    (tmp_path / "__pycache__" / "__init__.cpython-312.pyc").write_bytes(b"stale")
    (tmp_path / "__pycache__" / "other.cpython-312.pyc").write_bytes(b"kept")
    assert run(tool, path).returncode == 0
    assert sha256(path.read_bytes()) in (tmp_path / "__init__.py").read_text()
    assert sorted(os.listdir(tmp_path / "__pycache__")) == ["other.cpython-312.pyc"]


def test_strips_the_wheels_of_a_zipapp_and_updates_their_pin(tool, tmp_path):
    original = wheel()
    name = "virtualenv/seed/wheels/embed/pip-1.0-py3-none-any.whl"
    path = tmp_path / "virtualenv.pyz"
    path.write_bytes(archive({
        "__main__.py": b"",
        "virtualenv/seed/wheels/embed/__init__.py": f'BUNDLE_SHA256 = {{"pip": "{sha256(original)}"}}\n'.encode(),
        "virtualenv/seed/wheels/embed/__pycache__/__init__.cpython-312.pyc": b"stale",
        "virtualenv.dist-info/sboms/virtualenv.cdx.json": f'"{sha256(original)}"'.encode(),
        name: original,
    }, prefix=b"#!/usr/bin/env python3\n"))
    result = run(tool, path)
    assert result.returncode == 0, result.stderr
    data = path.read_bytes()
    assert data.startswith(b"#!/usr/bin/env python3\n")
    with zipfile.ZipFile(io.BytesIO(data)) as z:
        stripped = z.read(name)
        assert_clean(stripped)
        assert sha256(stripped) in z.read("virtualenv/seed/wheels/embed/__init__.py").decode()
        assert "virtualenv/seed/wheels/embed/__pycache__/__init__.cpython-312.pyc" not in z.namelist()
        # The SBOM keeps recording the upstream wheel
        assert sha256(original) in z.read("virtualenv.dist-info/sboms/virtualenv.cdx.json").decode()
    assert run(tool, path).returncode == 0
    assert path.read_bytes() == data


def test_strips_the_ensurepip_wheel_of_a_standard_library_zip(tool, tmp_path):
    path = tmp_path / "python312.zip"
    path.write_bytes(archive({"os.py": b"", "ensurepip/_bundled/pip-1.0-py3-none-any.whl": wheel()}))
    assert run(tool, path).returncode == 0
    with zipfile.ZipFile(path) as z:
        assert_clean(z.read("ensurepip/_bundled/pip-1.0-py3-none-any.whl"))


@pytest.mark.parametrize("newline", [b"\n", b"\r\n"])
def test_strips_a_pem_trust_list(tool, tmp_path, newline):
    def pem(*certificates):
        return b"\n".join(re.sub(rb"(?m)^#.*\n", b"", c) for c in certificates).replace(b"\n", newline)
    path = tmp_path / "codesignctl.pem"
    path.write_bytes(pem(EMSIGN_ECC_G3, TRUSTED, AAA_CERTIFICATE_SERVICES))
    assert run(tool, path).returncode == 0
    stripped = path.read_bytes()
    assert stripped.rstrip() == pem(TRUSTED).rstrip()  # a blank separator line may remain at the end
    assert stripped.count(b"\n") == stripped.count(newline)  # the line endings are kept


@pytest.mark.parametrize("name, data, error", [
    ("nope-*.whl", None, "Nothing found at"),
    ("notes.txt", b"", "Neither a wheel, an archive nor a PEM file"),
    ("bad.pem", b"-----BEGIN CERTIFICATE-----\n!!\n-----END CERTIFICATE-----\n", "Cannot parse the certificates"),
    ("pip-1.0-py3-none-any.whl", archive({"pip/__init__.py": b""}), "No pip/_vendor/certifi/cacert.pem"),
    ("tool.pyz", archive({"__main__.py": b""}), "No embedded pip wheel"),
])
def test_fails_on_what_it_does_not_recognize(tool, tmp_path, name, data, error):
    if data is not None:
        (tmp_path / name).write_bytes(data)
    result = run(tool, tmp_path / name)
    assert result.returncode != 0 and error in result.stderr


def test_fails_when_a_wheel_is_pinned_twice(tool, tmp_path):
    original = wheel()
    path = tmp_path / "pip-1.0-py3-none-any.whl"
    path.write_bytes(original)
    (tmp_path / "a.py").write_text(sha256(original))
    (tmp_path / "b.py").write_text(sha256(original))
    result = run(tool, path)
    assert result.returncode != 0 and "pinned more than once" in result.stderr
    assert path.read_bytes() == original

