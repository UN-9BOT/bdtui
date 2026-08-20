#!/usr/bin/env python3
"""
Append-only review-state log keyed by full commit SHA.

Usage:
    scripts/review/state.py append <sha> <verdict> [--reason T] [--pr N] [--headline T] [--round N]
    scripts/review/state.py latest <sha>
    scripts/review/state.py is-approved <sha>     # exit 0 if APPROVE present, 1 otherwise

File: .beads/.review-state.jsonl  (gitignored — local-only)
"""
from __future__ import annotations

import argparse
import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
STATE_PATH = ROOT / ".beads" / ".review-state.jsonl"

VERDICTS = {"APPROVE", "REQUEST_CHANGES", "PENDING", "PUSHED"}


def _now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


def _ensure_file() -> None:
    STATE_PATH.parent.mkdir(parents=True, exist_ok=True)
    if not STATE_PATH.exists():
        STATE_PATH.touch()


def cmd_append(args: argparse.Namespace) -> int:
    if args.verdict not in VERDICTS:
        print(f"bad verdict {args.verdict!r}, want one of {sorted(VERDICTS)}", file=sys.stderr)
        return 2
    _ensure_file()
    entry = {
        "ts": _now(),
        "sha": args.sha.strip(),
        "verdict": args.verdict,
        "reason": (args.reason or "").strip(),
        "pr": args.pr or 0,
        "headline": (args.headline or "").strip(),
        "round": args.round or 0,
    }
    with STATE_PATH.open("a", encoding="utf-8") as f:
        f.write(json.dumps(entry, ensure_ascii=False) + "\n")
    print(f"appended {entry['verdict']} for {entry['sha'][:12]} round={entry['round']}")
    return 0


def _entries_for(sha: str) -> list[dict]:
    if not STATE_PATH.exists():
        return []
    out: list[dict] = []
    needle = sha.strip().lower()
    for line in STATE_PATH.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            e = json.loads(line)
        except json.JSONDecodeError:
            continue
        if e.get("sha", "").lower().startswith(needle):
            out.append(e)
    return out


def cmd_latest(args: argparse.Namespace) -> int:
    entries = _entries_for(args.sha)
    if not entries:
        print("NONE")
        return 0
    e = entries[-1]
    print(json.dumps(e, ensure_ascii=False))
    return 0


def cmd_is_approved(args: argparse.Namespace) -> int:
    entries = _entries_for(args.sha)
    if any(e.get("verdict") == "APPROVE" for e in entries):
        print("APPROVED")
        return 0
    last = entries[-1] if entries else None
    if last:
        print(f"NOT_APPROVED (latest={last.get('verdict')})")
    else:
        print("NOT_APPROVED (no entries)")
    return 1


def main() -> int:
    ap = argparse.ArgumentParser()
    sub = ap.add_subparsers(dest="cmd", required=True)

    a = sub.add_parser("append")
    a.add_argument("sha")
    a.add_argument("verdict")
    a.add_argument("--reason", default="")
    a.add_argument("--pr", type=int, default=0)
    a.add_argument("--headline", default="")
    a.add_argument("--round", type=int, default=0)
    a.set_defaults(func=cmd_append)

    l = sub.add_parser("latest")
    l.add_argument("sha")
    l.set_defaults(func=cmd_latest)

    i = sub.add_parser("is-approved")
    i.add_argument("sha")
    i.set_defaults(func=cmd_is_approved)

    args = ap.parse_args()
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())