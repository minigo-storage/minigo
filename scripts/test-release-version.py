#!/usr/bin/env python3
"""Exercise release selection and stale/repeated source protection."""

import importlib.util
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("next-release-version.py")
spec = importlib.util.spec_from_file_location("release_version", SCRIPT)
release_version = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release_version)


class ReleaseVersionTests(unittest.TestCase):
    def test_initial_major_above_upstream(self):
        for increment in ("patch", "minor", "major"):
            self.assertEqual(release_version.next_version([], increment), ("3.0.0", None))
            self.assertEqual(release_version.next_version(["v2.0.4"], increment), ("3.0.0", None))

    def test_semantic_increments_and_numeric_order(self):
        tags = ["v3.9.9", "v3.10.2", "v3.10.1"]
        for increment, expected in (("patch", "3.10.3"), ("minor", "3.11.0"), ("major", "4.0.0")):
            self.assertEqual(release_version.next_version(tags, increment), (expected, "v3.10.2"))

    def test_unrelated_and_unstable_tags_do_not_advance_stable_version(self):
        tags = ["RELEASE.2025-10-15T17-29-55Z", "v99.0.0-rc.1", "v09.0.0", "v4.0", "4.0.0"]
        self.assertEqual(release_version.next_version(tags, "patch"), ("3.0.0", None))

    def test_invalid_increment(self):
        with self.assertRaises(ValueError):
            release_version.next_version([], "invalid")

    def test_git_history_guards(self):
        with tempfile.TemporaryDirectory() as directory:
            def git(*args):
                return subprocess.check_output(
                    ["git", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null",
                     "-C", directory, *args], text=True).strip()

            def select(*args):
                return subprocess.run([sys.executable, str(SCRIPT), "patch", *args], cwd=directory,
                                      text=True, capture_output=True)

            git("init", "-q")
            git("config", "user.name", "Release test")
            git("config", "user.email", "test@example.invalid")
            git("commit", "--allow-empty", "-qm", "Initial source")
            initial = git("rev-parse", "HEAD")
            self.assertEqual(select().stdout.strip(), "3.0.0")
            git("commit", "--allow-empty", "-qm", "First release source")
            git("tag", "v3.0.0")
            repeated = select()
            self.assertNotEqual(repeated.returncode, 0)
            self.assertIn("already reserved or released", repeated.stderr)
            resumed = select("3.0.0")
            self.assertEqual(resumed.returncode, 0, resumed.stderr)
            self.assertEqual(resumed.stdout.strip(), "3.0.0")
            git("commit", "--allow-empty", "-qm", "Fix")
            following = select()
            self.assertEqual(following.returncode, 0, following.stderr)
            self.assertEqual(following.stdout.strip(), "3.0.1")
            changed = select("3.1.0")
            self.assertNotEqual(changed.returncode, 0)
            self.assertIn("release version changed", changed.stderr)
            git("tag", "-a", "v3.1.0", "-m", "Newer release")
            outdated = select("3.0.0")
            self.assertNotEqual(outdated.returncode, 0)
            git("checkout", "-q", initial)
            stale = select()
            self.assertNotEqual(stale.returncode, 0)
            self.assertIn("must include the latest release", stale.stderr)


if __name__ == "__main__":
    unittest.main()
