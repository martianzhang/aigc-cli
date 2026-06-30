#!/usr/bin/env python3
"""Build ideas.json from multiple sources.

Usage:
    python scripts/convert_ideas.py                          # download NeXra + use CSVs in downloads/
    python scripts/convert_ideas.py --skip-download          # only CSVs, skip NeXra download

Deduplication key: source_url (Twitter/X post URL).
"""

import csv
import json
import os
import sys
import urllib.request

NEXRA_URL = "https://github.com/NeXra-AI/awesome-ai-image-prompts/raw/refs/heads/main/data/prompts.json"
NEXRA_IMAGE_PREFIX = "https://raw.githubusercontent.com/NeXra-AI/awesome-ai-image-prompts/refs/heads/main/"
OUTPUT = "cmd/ideas.json"
CSV_DIR = "downloads"
KEEP_LANGS = {"en", "zh"}


def log(msg):
    print(f"  {msg}")


def download_nexra() -> list:
    log(f"Downloading NeXra data...")
    resp = urllib.request.urlopen(NEXRA_URL)
    raw = resp.read().decode("utf-8")
    data = json.loads(raw)
    log(f"  {len(data)} entries")
    # Save raw copy to downloads/ for inspection
    os.makedirs("downloads", exist_ok=True)
    with open("downloads/prompts.json", "w", encoding="utf-8") as f:
        f.write(raw)
    log(f"  Saved raw copy to downloads/prompts.json")
    return convert_nexra(data)


def convert_nexra(data: list) -> list:
    result = []
    for r in data:
        if r.get("lang") not in KEEP_LANGS:
            continue
        entry = {
            "title": r.get("title", ""),
            "prompt": r.get("prompt", ""),
            "source_url": r.get("source_url", ""),
            "author": r.get("author", ""),
            "lang": r.get("lang", "en"),
        }
        if r.get("title_zh"):
            entry["title_zh"] = r["title_zh"]
        if r.get("prompt_zh"):
            entry["prompt_zh"] = r["prompt_zh"]
        if r.get("license"):
            entry["license"] = r["license"]
        img = r.get("image_url", "")
        if img:
            entry["image_urls"] = [NEXRA_IMAGE_PREFIX + img]
        result.append(entry)

    log(f"  Filtered (en/zh): {len(result)}")
    return result


def read_csvs() -> list:
    if not os.path.isdir(CSV_DIR):
        return []
    csv_files = sorted(f for f in os.listdir(CSV_DIR) if f.endswith(".csv"))
    if not csv_files:
        return []

    result = []
    for fn in csv_files:
        path = os.path.join(CSV_DIR, fn)
        count = 0
        with open(path, encoding="utf-8") as f:
            reader = csv.DictReader(f)
            for r in reader:
                content = r.get("content", "").strip()
                if not content:
                    continue

                # Detect language
                has_zh = any("\u4e00" <= c <= "\u9fff" for c in content)
                lang = "zh" if has_zh else "en"

                # Parse author
                author = ""
                try:
                    author = json.loads(r.get("author", "{}")).get("name", "")
                except json.JSONDecodeError:
                    author = r.get("author", "")

                # Parse image URLs
                images = []
                try:
                    images = json.loads(r.get("sourceMedia", "[]"))
                except json.JSONDecodeError:
                    pass

                entry = {
                    "title": r.get("title", ""),
                    "prompt": content,
                    "source_url": r.get("sourceLink", "").strip(),
                    "author": author,
                    "lang": lang,
                }
                if images:
                    entry["image_urls"] = images
                result.append(entry)
                count += 1
        log(f"  {fn}: {count} entries")
    return result


def merge(existing: list, *sources: list) -> list:
    seen = set()
    merged = []

    def dedup_key(entry: dict) -> str:
        """Use source_url as the primary unique identifier."""
        url = entry.get("source_url", "")
        if url:
            return url
        # Fallback for entries without a URL: use full prompt as key
        return entry.get("title", "") + "|" + entry.get("prompt", "")

    def sort_key(entry: dict) -> tuple:
        """Fully deterministic sort key for stable git diffs."""
        url = entry.get("source_url", "") or ""
        return (
            url,
            entry.get("lang", ""),
            entry.get("title", ""),
            entry.get("prompt", "")[:100],
        )

    for entry in existing:
        key = dedup_key(entry)
        if key in seen:
            continue
        seen.add(key)
        merged.append(entry)

    for source in sources:
        for entry in source:
            key = dedup_key(entry)
            if key in seen:
                continue
            seen.add(key)
            merged.append(entry)

    # Fully deterministic sort for stable git diffs
    merged.sort(key=sort_key)
    return merged


def main():
    skip_download = "--skip-download" in sys.argv
    force_update = "--update" in sys.argv

    # If file exists and not --update, skip entirely
    if os.path.exists(OUTPUT) and not force_update:
        log(f"{OUTPUT} exists, skipping (use --update to force rebuild)")
        return

    # Load existing (if any)
    existing = []
    if os.path.exists(OUTPUT):
        with open(OUTPUT, encoding="utf-8") as f:
            existing = json.load(f)
        log(f"Existing: {len(existing)} entries")

    # NeXra source
    nexra = []
    if not skip_download:
        nexra = download_nexra()
    elif os.path.exists("downloads/prompts.json"):
        log("Reading local downloads/prompts.json ...")
        with open("downloads/prompts.json", encoding="utf-8") as f:
            nexra_raw = json.load(f)
        nexra = convert_nexra(nexra_raw)
        log(f"  {len(nexra)} entries from local copy")
    else:
        log("Skipping NeXra download")

    # CSV sources
    csvs = read_csvs()

    # Merge
    merged = merge(existing, nexra, csvs)
    log(f"Merged: {len(merged)} entries (from {len(existing)} existing + {len(nexra)} nexra + {len(csvs)} csv)")

    with open(OUTPUT, "w", encoding="utf-8") as f:
        json.dump(merged, f, ensure_ascii=False, indent=2)
    log(f"Saved to {OUTPUT}")


if __name__ == "__main__":
    main()
