# Tools shared by the base images. A base that uses them starts with `FROM qodana-tools` after its global ARGs, which
# scripts/dockerfiles.py inlines as this stage and docker-bake.hcl resolves as the qodana-tools target, and copies what
# it needs with `COPY --from=qodana-tools`.
FROM scratch AS qodana-tools
COPY <<"EOF" /strip-distrusted-roots.py
# Removes the distrusted emSign ECC Root CA - G3 and AAA Certificate Services roots (QD-16607) from the certificate
# bundles that come with third-party software, for as long as their upstreams still ship them. It takes:
# - pip wheels, from the certifi bundle they vendor; the wheel's RECORD is updated to match
# - archives holding pip wheels: zipapps such as virtualenv-py3.pyz, and Python standard libraries such as python312.zip
# - PEM trust lists, such as the NuGet ones of the .NET SDK
# A changed wheel whose hash is pinned in Python code, as virtualenv's BUNDLE_SHA256 does, gets its pin updated: in the
# Python files next to a wheel on disk, in those of the archive for an embedded one. Metadata such as an SBOM keeps
# recording the upstream wheel. Files without the roots are left byte-identical.
# Anything it doesn't recognize fails the build, so that a changed layout cannot leave the roots in place.
# Usage: python3 -I strip-distrusted-roots.py WHEEL|ARCHIVE|PEM...
import base64, hashlib, io, os, re, stat, sys, zipfile

distrusted = {
    "86a1ecba089c4a8d3bbe2734c612ba341d813e043cf9e8a862cd5c57a36bbe6b",
    "d7a7a0fb5d7e2731d771e9484ebcdef71d5f0c3e0a2948782bc83ee0ea699ef4",
}
bundle_name = "pip/_vendor/certifi/cacert.pem"
# A certificate with its "# Issuer:" comment lines, as certifi has them, and the blank line after it
certificate = re.compile(rb"(?:#[^\r\n]*\r?\n)*-----BEGIN CERTIFICATE-----\r?\n([A-Za-z0-9+/=\r\n]*?)"
                         rb"-----END CERTIFICATE-----(?:\r?\n){0,2}")
embedded_pip_wheel = re.compile(r"(^|/)pip-[^/]+\.whl$")


def fingerprint(match):
    return hashlib.sha256(base64.b64decode(re.sub(rb"[\r\n]", b"", match.group(1)))).hexdigest()


def strip_pem(name, pem):
    """The PEM bundle without the distrusted roots."""
    certificates = list(certificate.finditer(pem))
    if not certificates or len(certificates) != pem.count(b"-----BEGIN CERTIFICATE-----"):
        sys.exit(f"Cannot parse the certificates in {name}")
    removed = [m for m in certificates if fingerprint(m) in distrusted]
    for m in reversed(removed):
        pem = pem[:m.start()] + pem[m.end():]
    if any(fingerprint(m) in distrusted for m in certificate.finditer(pem)):
        sys.exit(f"Distrusted roots left in {name}")
    print(f"{name}: removed {len(removed)} of {len(certificates)} certificates")
    return pem


def read_zip(data):
    """The part before the archive (a zipapp's shebang), the archive comment, the entries and their contents."""
    with zipfile.ZipFile(io.BytesIO(data)) as source:
        infos = source.infolist()
        contents = {info.filename: source.read(info) for info in infos}
        comment = source.comment
    return data[:min(info.header_offset for info in infos)], comment, infos, contents


def write_zip(prefix, comment, infos, contents):
    target = io.BytesIO(prefix)
    target.seek(len(prefix))
    with zipfile.ZipFile(target, "w") as archive:
        for info in infos:
            archive.writestr(info, contents[info.filename])
        archive.comment = comment
    return target.getvalue()


def strip_wheel(name, data):
    _, comment, infos, contents = read_zip(data)
    if bundle_name not in contents:
        sys.exit(f"No {bundle_name} in {name}, the layout has changed")
    bundle = strip_pem(f"{name}!/{bundle_name}", contents[bundle_name])
    if bundle == contents[bundle_name]:
        return data  # unchanged, so its hash still matches wherever it is pinned
    digest = base64.urlsafe_b64encode(hashlib.sha256(bundle).digest()).rstrip(b"=").decode()
    record = next(entry for entry in contents if entry.endswith(".dist-info/RECORD"))
    contents[bundle_name] = bundle
    contents[record], replaced = re.subn(rb"^" + re.escape(bundle_name.encode()) + rb",[^\r\n]*",
                                         f"{bundle_name},sha256={digest},{len(bundle)}".encode(), contents[record],
                                         flags=re.M)
    if replaced != 1:
        sys.exit(f"Expected one {bundle_name} line in {record} of {name}, found {replaced}")
    return write_zip(b"", comment, infos, contents)


def repin(name, old, new, sources):
    """The source ({path: content}) pinning the hash of the changed wheel, with the pin updated, or None."""
    old, new = hashlib.sha256(old).hexdigest().encode(), hashlib.sha256(new).hexdigest().encode()
    pins = {source: content.count(old) for source, content in sources.items() if old in content}
    if not pins:
        return None
    if sum(pins.values()) != 1:
        sys.exit(f"The hash of {name} is pinned more than once {pins}, the layout has changed")
    (source,) = pins
    print(f"{name}: updated its pinned hash in {source}")
    return source, sources[source].replace(old, new)


def bytecode_of(source, names):
    """The cached bytecode of a Python source among names, which would still hold the old pin."""
    folder, _, file = source.rpartition("/")
    cached = re.compile(re.escape(f"{folder}/" if folder else "") + re.escape(f"__pycache__/{file[:-3]}.") + r"[^/]+\.pyc$")
    return {n for n in names if cached.match(n) or n == source + "c"}


def write(path, data):
    """Replaces the file atomically, keeping its mode and owner."""
    st = os.stat(path)
    with open(path + ".new", "wb") as f:
        f.write(data)
    os.chmod(path + ".new", stat.S_IMODE(st.st_mode))
    if (os.stat(path + ".new").st_uid, os.stat(path + ".new").st_gid) != (st.st_uid, st.st_gid):
        os.chown(path + ".new", st.st_uid, st.st_gid)
    os.replace(path + ".new", path)


def files_in(folder):
    return [os.path.join(root, f) for root, _, files in os.walk(folder) for f in files]


# An unmatched shell glob arrives as the pattern itself, which then does not exist
missing = [p for p in sys.argv[1:] if not os.path.isfile(p)]
if not sys.argv[1:] or missing:
    sys.exit(f"Nothing found at {' '.join(missing) or 'the given paths'}, the layout has changed")
for path in sorted({os.path.realpath(p) for p in sys.argv[1:]}):
    with open(path, "rb") as f:
        data = original = f.read()
    if path.endswith(".whl"):
        data = strip_wheel(path, original)
        if data != original:
            folder = os.path.dirname(path)
            sources = {}
            for entry in sorted(os.listdir(folder)):
                if entry.endswith(".py"):
                    with open(os.path.join(folder, entry), "rb") as f:
                        sources[os.path.join(folder, entry)] = f.read()
            pinned = repin(path, original, data, sources)
            if pinned:
                write(*pinned)
                for cached in bytecode_of(pinned[0], files_in(os.path.join(folder, "__pycache__"))):
                    os.remove(cached)
    elif path.endswith((".pyz", ".zip")):
        prefix, comment, infos, contents = read_zip(original)
        wheels = [name for name in contents if embedded_pip_wheel.search(name)]
        if not wheels:
            sys.exit(f"No embedded pip wheel found in {path}, the layout has changed")
        changed = False
        for name in wheels:
            stripped = strip_wheel(f"{path}!/{name}", contents[name])
            if stripped == contents[name]:
                continue
            # Only code pins; metadata such as an SBOM keeps recording the upstream wheel
            others = {n: c for n, c in contents.items() if n.endswith(".py")}
            pinned = repin(f"{path}!/{name}", contents[name], stripped, others)
            if pinned:
                contents[pinned[0]] = pinned[1]
                stale = bytecode_of(pinned[0], contents)
                infos = [info for info in infos if info.filename not in stale]
            contents[name] = stripped
            changed = True
        if changed:
            data = write_zip(prefix, comment, infos, contents)
    elif path.endswith(".pem"):
        data = strip_pem(path, original)
    else:
        sys.exit(f"Neither a wheel, an archive nor a PEM file: {path}")
    if data != original:
        write(path, data)
EOF
