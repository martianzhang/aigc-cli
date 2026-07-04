#!/usr/bin/env python3
"""Build or deduplicate ideas.json.

Always ensures:
  - No duplicates (source_url / image / exact content / normalized text)
  - Stable ordering: existing entries keep position, new entries append at end

Commands:
  (default)  Dedup + merge new sources (downloads/*.json, *.csv) into ideas.json
  --dedup    Dedup only, skip source merge

Usage:
  python scripts/convert_ideas.py             # dedup + merge sources
  python scripts/convert_ideas.py --dedup     # just dedup
"""

import csv, hashlib, json, os, re
from urllib.parse import urlparse

NEXRA_PREFIX = "https://raw.githubusercontent.com/NeXra-AI/awesome-ai-image-prompts/refs/heads/main/"
OUTPUT = "docs/ideas.json"
CSV_DIR = "downloads"
KEEP_LANGS = {"en", "zh"}


def log(msg):
    print(f"  {msg}")


def normalize(entry: dict) -> dict | None:
    """Normalize a raw entry. Returns None if prompt is missing or lang excluded."""
    prompt = (entry.get("prompt") or "").strip()
    if not prompt:
        return None
    lang = entry.get("lang", "")
    if lang and lang not in KEEP_LANGS:
        return None

    out = {
        "title": (entry.get("title") or "").strip(),
        "prompt": prompt,
        "source_url": (entry.get("source_url") or "").strip(),
        "author": (entry.get("author") or "").strip(),
        "lang": lang or "en",
    }
    img = entry.get("image_url", "")
    imgs = entry.get("image_urls") or []
    if img and not imgs:
        imgs = [NEXRA_PREFIX + img]
    if imgs:
        out["image_urls"] = imgs
    for f in ("title_zh", "prompt_zh", "license"):
        if entry.get(f):
            out[f] = entry[f]
    return out


def read_file(path: str) -> list:
    """Read a single file and return normalized entries. Tries JSON then CSV."""
    basename = os.path.basename(path)
    # Try JSON first
    try:
        with open(path, encoding="utf-8") as f:
            raw = json.load(f)
        if isinstance(raw, list):
            before = len(raw)
            entries = [normalize(e) for e in raw]
            entries = [e for e in entries if e]
            log(f"  {basename}: {len(entries)}/{before} entries")
            return entries
        log(f"  {basename}: skipped (type={type(raw).__name__})")
        return []
    except (json.JSONDecodeError, UnicodeDecodeError):
        pass
    # Fallback: CSV
    try:
        with open(path, encoding="utf-8") as f:
            reader = csv.DictReader(f)
            if "content" not in (reader.fieldnames or []):
                return []  # binary/image files and non-csv files, skip silently
            entries = []
            for r in reader:
                content = (r.get("content") or "").strip()
                if not content:
                    continue
                has_zh = any("\u4e00" <= c <= "\u9fff" for c in content)
                author = r.get("author", "")
                try:
                    author = json.loads(author).get("name", "")
                except json.JSONDecodeError:
                    pass
                images = []
                try:
                    images = json.loads(r.get("sourceMedia", "[]"))
                except json.JSONDecodeError:
                    pass
                e = {
                    "title": r.get("title", ""),
                    "prompt": content,
                    "source_url": (r.get("sourceLink") or "").strip(),
                    "author": author,
                    "lang": "zh" if has_zh else "en",
                }
                if images:
                    e["image_urls"] = images
                entries.append(e)
            log(f"  {basename}: {len(entries)} entries")
            return entries
    except Exception:
        return []


def read_sources() -> list:
    """Scan downloads/ and read all source files, return normalized entries."""
    if not os.path.isdir(CSV_DIR):
        return []
    data_exts = {".json", ".csv"}
    files = sorted(
        f for f in os.listdir(CSV_DIR)
        if os.path.isfile(os.path.join(CSV_DIR, f))
        and os.path.splitext(f)[1].lower() in data_exts
    )
    entries = []
    for fn in files:
        entries.extend(read_file(os.path.join(CSV_DIR, fn)))
    return entries


def text_key(text: str) -> str:
    """Normalized text key for dedup (first 200 chars)."""
    t = re.sub(r"[^\w\s]", "", text.lower().strip())
    return re.sub(r"\s+", " ", t)[:200]


def content_hash(entry: dict) -> str:
    """Stable hash of entry content for ordering."""
    raw = json.dumps(entry, sort_keys=True, ensure_ascii=False)
    return hashlib.md5(raw.encode()).hexdigest()


def dedup(entries: list) -> list:
    """Remove internal duplicates from a list of entries.

    Keeps the FIRST occurrence of each unique entry (identified by
    source_url, image filenames, exact content, or normalized text).
    Order is preserved — only duplicates are removed.
    """
    seen_urls: set[str] = set()
    seen_imgs: set[str] = set()
    seen_content: set[str] = set()
    seen_prompts: set[str] = set()

    result = []
    for e in entries:
        url = e.get("source_url", "")
        imgs = e.get("image_urls", [])
        dup = False

        if url and url in seen_urls:
            dup = True
        if not dup:
            for u in imgs:
                if os.path.basename(urlparse(u).path) in seen_imgs:
                    dup = True
                    break
        if not dup:
            key = e.get("title", "") + "|" + e.get("prompt", "")
            if key in seen_content:
                dup = True
        if not dup:
            norm = text_key(e.get("prompt", ""))
            if norm and norm in seen_prompts:
                dup = True

        if dup:
            continue

        # Mark as seen
        if url:
            seen_urls.add(url)
        for u in imgs:
            seen_imgs.add(os.path.basename(urlparse(u).path))
        seen_content.add(e.get("title", "") + "|" + e.get("prompt", ""))
        norm = text_key(e.get("prompt", ""))
        if norm:
            seen_prompts.add(norm)

        result.append(e)

    return result


def merge(existing: list, *sources: list) -> list:
    """Merge new entries into existing.

    - Existing entries keep their original order (zero movement in git diff).
    - New entries are appended at the end, sorted deterministically.
    """
    seen = set()  # hashes of all entries already kept

    def hash_entry(e: dict) -> str:
        url = e.get("source_url", "")
        imgs = tuple(os.path.basename(urlparse(u).path) for u in e.get("image_urls", []))
        key = e.get("title", "") + "|" + e.get("prompt", "")
        norm = text_key(e.get("prompt", ""))
        return (url, imgs, key, norm)

    for e in existing:
        seen.add(hash_entry(e))

    new_accepted: list[dict] = []
    for src in sources:
        for e in src:
            h = hash_entry(e)
            if h in seen:
                continue
            seen.add(h)
            new_accepted.append(e)

    new_accepted.sort(key=lambda e: (
        e.get("source_url", "") or "",
        e.get("lang", ""),
        e.get("title", ""),
        content_hash(e),
    ))

    return existing + new_accepted


def main():
    import sys
    args = set(sys.argv[1:])

    if not os.path.exists(OUTPUT):
        log(f"{OUTPUT} not found")
        return

    with open(OUTPUT, encoding="utf-8") as f:
        entries = json.load(f)
    log(f"Loaded: {len(entries)} entries")

    # Step 1: Always dedup — remove any accumulated internal duplicates
    before = len(entries)
    entries = dedup(entries)
    if len(entries) < before:
        log(f"Dedup: removed {before - len(entries)} internal duplicates")

    # Step 2: Merge new sources (or stop if --dedup-only)
    if "--dedup" in args:
        if len(entries) == before:
            log("No duplicates found")
    else:
        new_entries = read_sources()
        if not new_entries:
            log(f"No source files with valid entries in {CSV_DIR}/")
            return
        # Dedup new sources among themselves before merging
        before_dedup = len(new_entries)
        new_entries = dedup(new_entries)
        if len(new_entries) < before_dedup:
            log(f"Source dedup: removed {before_dedup - len(new_entries)} cross-source duplicates")
        before = len(entries)
        entries = merge(entries, new_entries)
        log(f"Merged: +{len(entries) - before} new entries")

    with open(OUTPUT, "w", encoding="utf-8") as f:
        json.dump(entries, f, ensure_ascii=False, indent=2)
    log(f"Saved to {OUTPUT} ({len(entries)} entries)")


if __name__ == "__main__":
    main()
