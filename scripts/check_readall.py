#!/usr/bin/env python3
"""Enforce P2: every io.ReadAll call is capped or explicitly justified.

A call site passes when either:
  * it wraps its reader in io.LimitReader on the same statement, or
  * the preceding non-blank line is a `//` comment explaining why an unbounded
    read is safe (stdin, local file, already-capped buffer).

Test files are skipped: they only read fixtures.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CALL_RE = re.compile(r"\bio\.ReadAll\(")

# The helper itself is the bounded reader.
SKIP_FILES = {"internal/httpx/retry.go"}


def go_files() -> list[Path]:
    files: list[Path] = []
    for path in ROOT.rglob("*.go"):
        if ".git" in path.parts or "vendor" in path.parts:
            continue
        if path.name.endswith("_test.go"):
            continue
        rel = path.relative_to(ROOT).as_posix()
        if rel in SKIP_FILES:
            continue
        files.append(path)
    return files


def check(path: Path) -> list[str]:
    errors: list[str] = []
    lines = path.read_text(encoding="utf-8", errors="replace").splitlines()
    for idx, line in enumerate(lines):
        if not CALL_RE.search(line):
            continue
        if "io.LimitReader(" in line:
            continue
        prev = idx - 1
        while prev >= 0 and not lines[prev].strip():
            prev -= 1
        if prev >= 0 and lines[prev].lstrip().startswith("//"):
            continue
        errors.append(f"{path.relative_to(ROOT)}:{idx + 1}: io.ReadAll without a limit or justification comment")
    return errors


def main() -> int:
    errors: list[str] = []
    for path in go_files():
        errors.extend(check(path))
    if errors:
        print("io.ReadAll check failed:")
        for err in errors:
            print("  -", err)
        print("Wrap the reader in io.LimitReader or add a `//` justification line.")
        return 1
    print(f"io.ReadAll check passed ({len(go_files())} non-test Go files)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
