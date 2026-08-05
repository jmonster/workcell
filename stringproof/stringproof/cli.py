from __future__ import annotations

import argparse
import json
from pathlib import Path
import sys
from typing import Sequence

from .audit import AuditError, SCHEMA_VERSION, check


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="stringproof",
        description="Check Apple .strings catalogs for structural and linguistic corruption.",
    )
    subparsers = parser.add_subparsers(dest="command", required=True)
    check_parser = subparsers.add_parser(
        "check",
        help="check catalogs beneath a root directory",
    )
    check_parser.add_argument("root", type=Path)
    check_parser.add_argument("--source-locale", required=True)
    check_parser.add_argument("--json", action="store_true", dest="json_output")
    return parser


def _error_payload(message: str) -> dict[str, object]:
    return {
        "schema_version": SCHEMA_VERSION,
        "status": "error",
        "findings": [],
        "summary": {
            "files": 0,
            "locales": 0,
            "findings": 0,
            "errors": 0,
            "warnings": 0,
            "by_code": {},
        },
        "error": message,
    }


def _write_json(payload: dict[str, object]) -> bool:
    try:
        sys.stdout.write(json.dumps(payload, ensure_ascii=False, indent=2) + "\n")
        return True
    except Exception as error:
        sys.stderr.write(f"stringproof: cannot produce JSON: {error}\n")
        return False


def main(argv: Sequence[str] | None = None) -> int:
    args = _parser().parse_args(argv)
    try:
        report = check(args.root, args.source_locale)
    except AuditError as error:
        message = str(error)
        if args.json_output:
            return 2 if _write_json(_error_payload(message)) else 2
        sys.stderr.write(f"stringproof: {message}\n")
        return 2
    except Exception as error:
        message = f"internal failure: {error}"
        if args.json_output:
            return 2 if _write_json(_error_payload(message)) else 2
        sys.stderr.write(f"stringproof: {message}\n")
        return 2

    if args.json_output:
        if not _write_json(report):
            return 2
    else:
        for finding in report["findings"]:
            sys.stdout.write(
                f"{finding['path']}:{finding['line']} "
                f"[{finding['code']}] {finding['locale']} {finding['key']}: "
                f"{finding['message']}\n"
            )
    return 1 if report["findings"] else 0


if __name__ == "__main__":
    raise SystemExit(main())

