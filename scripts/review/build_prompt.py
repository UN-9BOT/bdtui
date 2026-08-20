#!/usr/bin/env python3
"""
Build a ChatGPT review prompt that mandates the REVIEW_END <full-sha> marker.

Usage:
    scripts/review/build_prompt.py <full-sha> [--pr N] [--title T] [--summary S]

Always uses --require-marker to force the reviewer to emit a marker line.
"""
from __future__ import annotations

import argparse
import sys
import textwrap
from pathlib import Path


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("sha", help="full 40-char commit SHA")
    p.add_argument("--pr", type=int, default=0, help="PR number")
    p.add_argument("--title", default="(no title)", help="commit/PR title")
    p.add_argument(
        "--summary",
        default="",
        help="short diff summary (files + tests + behavioural notes)",
    )
    p.add_argument(
        "--marker-only",
        action="store_true",
        help="short follow-up: just ask for the marker",
    )
    args = p.parse_args()

    sha = args.sha.strip()
    if len(sha) < 7:
        print(f"bad sha: {sha}", file=sys.stderr)
        return 2

    if args.marker_only:
        out = (
            f"PR #{args.pr} (HEAD = {sha}). "
            "If your previous review is finished, respond with exactly "
            f"`REVIEW_END {sha} APPROVE` or `REVIEW_END {sha} REQUEST_CHANGES: <one-line reason>`. "
            "Nothing else."
        )
        print(out)
        return 0

    title = args.title.strip()
    summary = args.summary.strip() or "(no summary provided)"

    out = textwrap.dedent(
        f"""
        You are reviewing PR #{args.pr} (HEAD = {sha}, title: "{title}").

        Context / diff summary:
        {summary}

        Reviewer protocol (mandatory):
        1. Read the PR diff at https://github.com/UN-9BOT/bdtui/pull/{args.pr}/files (commit {sha}).
        2. Treat findings as P0 (data loss / security / wrong contract), P1 (correctness bug), P2 (style / nits).
        3. End your response with EXACTLY ONE marker line, no quotes, no markdown:
           REVIEW_END {sha} APPROVE
           OR
           REVIEW_END {sha} REQUEST_CHANGES
           followed by a single-line bulleted list of P0/P1 items (P2 are optional, list only what you actually want changed).
        4. If you cannot finish in one pass, end with NO marker and say so; I will follow up with `--marker-only`.

        Do not add any text after the marker line.
        """
    ).strip() + "\n"

    print(out)
    return 0


if __name__ == "__main__":
    sys.exit(main())