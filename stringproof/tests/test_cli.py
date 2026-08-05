from __future__ import annotations

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


REPOSITORY_ROOT = Path(__file__).resolve().parents[1]


def run_cli(*arguments: str) -> subprocess.CompletedProcess[str]:
    environment = os.environ.copy()
    environment["PYTHONPATH"] = str(REPOSITORY_ROOT)
    return subprocess.run(
        [sys.executable, "-m", "stringproof", *arguments],
        cwd=REPOSITORY_ROOT,
        env=environment,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )


class CliTests(unittest.TestCase):
    def test_clean_human_output_is_quiet_and_exit_zero(self) -> None:
        result = run_cli(
            "check",
            "fixtures/clean",
            "--source-locale",
            "en",
        )
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout, "")
        self.assertEqual(result.stderr, "")

    def test_human_findings_have_the_stable_line_shape(self) -> None:
        result = run_cli(
            "check",
            "fixtures/corrupted",
            "--source-locale",
            "en",
        )
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stderr, "")
        for line in result.stdout.splitlines():
            self.assertRegex(
                line,
                r"^[^:]+\.strings:\d+ \[[a-z.]+\] [^ ]+ [^:]+: .+$",
            )

    def test_json_shape_and_exit_one(self) -> None:
        result = run_cli(
            "check",
            "fixtures/corrupted",
            "--source-locale",
            "en",
            "--json",
        )
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stderr, "")
        payload = json.loads(result.stdout)
        self.assertEqual(
            set(payload),
            {"schema_version", "status", "findings", "summary"},
        )
        self.assertEqual(payload["schema_version"], 1)
        self.assertEqual(payload["status"], "findings")
        self.assertEqual(
            set(payload["findings"][0]),
            {"code", "severity", "path", "line", "locale", "key", "message"},
        )
        self.assertEqual(
            set(payload["summary"]),
            {"files", "locales", "findings", "errors", "warnings", "by_code"},
        )

    def test_invalid_root_and_unreadable_encoding_exit_two(self) -> None:
        missing = run_cli(
            "check",
            "fixtures/does-not-exist",
            "--source-locale",
            "en",
        )
        self.assertEqual(missing.returncode, 2)
        self.assertEqual(missing.stdout, "")
        self.assertIn("stringproof:", missing.stderr)

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            source = root / "en.lproj" / "Localizable.strings"
            source.parent.mkdir()
            source.write_bytes(b"\xff\xff\xff")
            unreadable = run_cli(
                "check",
                str(root),
                "--source-locale",
                "en",
                "--json",
            )
        self.assertEqual(unreadable.returncode, 2)
        self.assertEqual(unreadable.stderr, "")
        payload = json.loads(unreadable.stdout)
        self.assertEqual(payload["status"], "error")
        self.assertEqual(payload["findings"], [])

    def test_invalid_invocation_exits_two(self) -> None:
        result = run_cli("check")
        self.assertEqual(result.returncode, 2)
        self.assertEqual(result.stdout, "")
        self.assertIn("usage:", result.stderr)


if __name__ == "__main__":
    unittest.main()

