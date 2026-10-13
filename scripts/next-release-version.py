#!/usr/bin/env python3
"""Select the next stable Minigo version from the repository's release tags."""

import re
import subprocess
import sys


# One major version above the bundled upstream console's v2.0.4 release.
INITIAL_VERSION = (3, 0, 0)
VERSION_TAG = re.compile(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)")


def next_version(tags, increment):
    if increment not in ("patch", "minor", "major"):
        raise ValueError("increment must be patch, minor or major")
    versions = [tuple(map(int, match.groups())) for tag in tags
                if (match := VERSION_TAG.fullmatch(tag))]
    latest = max(versions, default=None)
    if latest is None or latest < INITIAL_VERSION:
        return ".".join(map(str, INITIAL_VERSION)), None
    major, minor, patch = latest
    if increment == "major":
        version = (major + 1, 0, 0)
    elif increment == "minor":
        version = (major, minor + 1, 0)
    else:
        version = (major, minor, patch + 1)
    return ".".join(map(str, version)), "v" + ".".join(map(str, latest))


def main():
    if len(sys.argv) not in (2, 3):
        raise ValueError("usage: next-release-version.py patch|minor|major [expected-version]")
    expected = sys.argv[2] if len(sys.argv) == 3 else None
    tags = subprocess.check_output(["git", "tag", "--list"], text=True).splitlines()
    version, previous = next_version(tags, sys.argv[1])
    if previous:
        head = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
        released = subprocess.check_output(
            ["git", "rev-parse", f"{previous}^{{commit}}"], text=True).strip()
        if head == released:
            if expected == previous.removeprefix("v"):
                # A failed publish job may resume its reserved version, using
                # the same tested artifacts and source commit.
                print(expected)
                return
            raise ValueError(f"this commit is already reserved or released as {previous}")
        result = subprocess.run(["git", "merge-base", "--is-ancestor", previous, head])
        if result.returncode:
            raise ValueError(f"HEAD must include the latest release {previous}")
    if expected is not None and expected != version:
        raise ValueError(f"release version changed from {expected} to {version}; start a new run")
    print(version)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, subprocess.CalledProcessError) as error:
        sys.exit(str(error))
