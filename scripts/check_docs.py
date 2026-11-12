#!/usr/bin/env python3
"""Check published documentation links and the zh/en mirror.

Checks:
  1. Markdown links in README*.md and docs/{zh,en}/**/*.md resolve to a file.
  2. Repo-relative ``docs/.../*.md`` path tokens in the README files exist.
  3. docs/zh and docs/en contain the same set of files.

Local design notes (docs/design) and historical release notes are skipped:
their examples intentionally contain non-file snippets and old paths.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

# Markdown link target: [text](target) or [text](target "title")
LINK_RE = re.compile(r"\[[^\]]*\]\(([^)\s]+)(?:\s+[^)]*)?\)")
# Repo-relative doc path in prose / ASCII trees: docs/foo/bar.md
DOC_PATH_RE = re.compile(r"\bdocs/[A-Za-z0-9_./-]+\.md\b")

# Files/directories whose links are not part of the published docs.
SKIP_DIRS = {"design", "release_notes", ".git", "node_modules"}


def published_markdown() -> list[Path]:
    files = [ROOT / "README.md", ROOT / "README_zh.md"]
    for lang in ("zh", "en"):
        base = ROOT / "docs" / lang
        if base.is_dir():
            files.extend(base.rglob("*.md"))
    return [p for p in files if p.is_file()]


def iter_docs_markdown() -> list[Path]:
    files: list[Path] = []
    for base in (ROOT / "docs" / "zh", ROOT / "docs" / "en"):
        if not base.is_dir():
            continue
        for p in base.rglob("*.md"):
            if any(part in SKIP_DIRS for part in p.relative_to(ROOT).parts):
                continue
            files.append(p)
    return files


def is_external(target: str) -> bool:
    return (
        target.startswith(("http://", "https://", "mailto:", "#"))
        or target.startswith("data:")
    )


def check_link(source: Path, target: str) -> str | None:
    target = target.split("#", 1)[0]
    if not target or is_external(target):
        return None
    resolved = (source.parent / target).resolve()
    if not resolved.exists():
        return f"{source.relative_to(ROOT)}: broken link -> {target}"
    return None


def check_readme_paths(source: Path, text: str) -> list[str]:
    errors: list[str] = []
    for match in DOC_PATH_RE.finditer(text):
        token = match.group(0).rstrip(".,;:")
        if not (ROOT / token).exists():
            errors.append(f"{source.relative_to(ROOT)}: missing doc path -> {token}")
    return errors


def strip_code(text: str) -> str:
    """Remove fenced blocks and inline code so examples are not link-checked."""
    text = re.sub(r"```.*?```", "", text, flags=re.S)
    text = re.sub(r"`[^`]*`", "", text)
    return text


def check_links() -> list[str]:
    errors: list[str] = []
    for path in published_markdown():
        text = path.read_text(encoding="utf-8", errors="replace")
        for match in LINK_RE.finditer(strip_code(text)):
            err = check_link(path, match.group(1))
            if err:
                errors.append(err)
        if path.name.startswith("README"):
            errors.extend(check_readme_paths(path, text))
    return errors


def rel_files(base: Path) -> set[str]:
    if not base.is_dir():
        return set()
    return {
        str(p.relative_to(base))
        for p in base.rglob("*")
        if p.is_file()
    }


def check_parity() -> list[str]:
    zh = rel_files(ROOT / "docs" / "zh")
    en = rel_files(ROOT / "docs" / "en")
    errors: list[str] = []
    for name in sorted(zh - en):
        errors.append(f"docs/zh/{name}: missing English counterpart docs/en/{name}")
    for name in sorted(en - zh):
        errors.append(f"docs/en/{name}: missing Chinese counterpart docs/zh/{name}")
    return errors


def main() -> int:
    errors = check_links() + check_parity()
    # De-duplicate while keeping order.
    seen: set[str] = set()
    unique = [e for e in errors if not (e in seen or seen.add(e))]

    if unique:
        print("documentation check failed:")
        for err in unique:
            print("  -", err)
        return 1

    print(f"documentation check passed ({len(published_markdown())} files)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
