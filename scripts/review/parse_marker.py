#!/usr/bin/env python3
"""
Parse a ChatGPT snapshot / transcript dump for the REVIEW_END <sha> marker.

Reads text from stdin or from a file path. Prints:
    APPROVE
    REQUEST_CHANGES
    PENDING            (no marker found)
plus, when REQUEST_CHANGES, the bullet list that follows the marker.

Usage:
    scripts/review/parse_marker.py <expected-sha>                # stdin
    scripts/review/parse_marker.py <expected-sha> <snapshot.txt>
    cat snapshot.txt | scripts/review/parse_marker.py <expected-sha>
"""
from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

MARKER = re.compile(r"^REVIEW_END\s+([0-9a-f]{7,40})\s+(APPROVE|REQUEST_CHANGES)\s*(.*)$")


def parse(text: str, expected_sha: str) -> tuple[str, str]:
    expected = expected_sha.strip().lower()
    last_status = "PENDING"
    last_reasons: list[str] = []

    for line in text.splitlines():
        line = line.rstrip("\r")
        m = MARKER.match(line.strip())
        if not m:
            continue
        sha, status, tail = m.group(1).lower(), m.group(2), m.group(3).strip()
        if expected and not sha.startswith(expected):
            continue
        last_status = status
        if status == "REQUEST_CHANGES":
            last_reasons = [tail] if tail else []
        else:
            last_reasons = []

    if last_status == "REQUEST_CHANGES":
        return last_status, "\n".join(last_reasons).strip()
    return last_status, ""


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("sha")
    ap.add_argument("file", nargs="?", default="-")
    ap.add_argument(
        "--quiet",
        action="store_true",
        help="print only the status word (APPROVE/REQUEST_CHANGES/PENDING)",
    )
    args = ap.parse_args()

    if args.file == "-":
        text = sys.stdin.read()
    else:
        text = Path(args.file).read_text(encoding="utf-8", errors="replace")

    status, reasons = parse(text, args.sha)
    if args.quiet:
        print(status)
    else:
        if reasons:
            print(f"{status}\n{reasons}")
        else:
            print(status)
    return 0 if status in ("APPROVE", "REQUEST_CHANGES", "PENDING") else 1


if __name__ == "__main__":
    sys.exit(main())