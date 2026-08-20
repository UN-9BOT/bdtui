#!/usr/bin/env python3
"""
Save a ChatGPT snapshot dump to .beads/.review-snapshots/<sha>-<round>.txt
and append a PENDING entry to the review state log so we can detect retries.

Usage:
    scripts/review/snapshot_save.py <sha> <round>   # stdin → file
    scripts/review/snapshot_save.py <sha> <round> --file path/to/snapshot.txt
"""
from __future__ import annotations

import argparse
import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DIR = ROOT / ".beads" / ".review-snapshots"
STATE = ROOT / ".beads" / ".review-state.jsonl"


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("sha")
    ap.add_argument("round", type=int)
    ap.add_argument("--file", default="-")
    args = ap.parse_args()

    DIR.mkdir(parents=True, exist_ok=True)
    if args.file == "-":
        text = sys.stdin.read()
    else:
        text = Path(args.file).read_text(encoding="utf-8", errors="replace")

    out = DIR / f"{args.sha[:12]}-r{args.round}.txt"
    out.write_text(text, encoding="utf-8")
    print(f"saved {out} ({len(text)} bytes)")

    STATE.parent.mkdir(parents=True, exist_ok=True)
    with STATE.open("a", encoding="utf-8") as f:
        f.write(
            f'{{"ts":"{__import__("datetime").datetime.utcnow().isoformat()}Z",'
            f'"sha":"{args.sha}","verdict":"SNAPSHOT","round":{args.round},'
            f'"path":"{out.relative_to(ROOT)}"}}\n'
        )
    return 0


if __name__ == "__main__":
    sys.exit(main())