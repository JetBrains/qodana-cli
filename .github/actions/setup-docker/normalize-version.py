# /// script
# requires-python = "==3.13.15"
# dependencies = [
#     "htmllistparse==0.6.1",
#     "natsort==8.4.0",
# ]
# ///
import json
import platform
import re
import sys
import os
from urllib.request import urlopen

import htmllistparse
from natsort import natsorted


def fail(message):
    print(message, file=sys.stderr)
    sys.exit(1)


def latest_binary(version, os=None, arch=None):
    """For a given X or X.Y version, find the latest X.Y.Z released binary."""
    def host_os():
        RENAME = {
            "Darwin": "mac",
            "Linux": "linux",
            "Windows": "win",
        }

        return RENAME[platform.system()]


    def host_arch():
        RENAME = {
            "AMD64": "x86_64",
            "x86_64": "x86_64",
            "arm64": "aarch64",
        }

        return RENAME[platform.machine()]

    os = os or host_os()
    arch = arch or host_arch()

    re_target_binary_name = re.compile(fr"docker-({re.escape(version)}(?:\.\d+)*)")

    _, binary_releases = htmllistparse.fetch_listing(f"https://download.docker.com/{os}/static/stable/{arch}/")
    matching_versions = []
    for binary_release in binary_releases:
        filename = binary_release.name
        if m := re_target_binary_name.match(filename):
            matching_versions.append(m[1])

    matching_versions = natsorted(matching_versions)
    if len(matching_versions) == 0:
        fail(f"No binary release found for version '{version}'")
    return matching_versions[-1]


query = os.getenv("QUERY") or sys.argv[1]
if query == "latest":
    print(query)
elif m := re.fullmatch(r"v?(\d+\.\d+\.\d+)", query):  # Full version specified
    print(f"v{m[1]}")
elif m := re.fullmatch(r"v?(\d+(?:\.\d+)?)", query):  # One or two semver components omitted
    print("v", latest_binary(m[1]), sep="")
else:
    fail(f"Cannot normalize version '{query}'")
