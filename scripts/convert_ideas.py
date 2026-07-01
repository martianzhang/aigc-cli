#!/usr/bin/env python3
"""Build ideas.json from local source files.

Required source files (in downloads/):

  prompts.json (NeXra Awesome AI Image Prompts, GET)
      curl -sL 'https://github.com/NeXra-AI/awesome-ai-image-prompts/raw/refs/heads/main/data/prompts.json' \
        -o downloads/prompts.json

  gpt-image-2-{date}.csv (Youmind model prompts, POST — -d implies POST)
      curl -s 'https://youmind.com/youmarketing-api/prompts-download?model=gpt-image-2' \
        -b downloads/youmind.com_cookies.txt \
        -H 'content-type: application/json' \
        -d '{"model":"gpt-image-2"}' \
        -o downloads/gpt-image-2-$(date +%Y%m%d).csv

  nano-banana-pro-{date}.csv (POST)
      curl -s 'https://youmind.com/youmarketing-api/prompts-download?model=nano-banana-pro' \
        -b downloads/youmind.com_cookies.txt \
        -H 'content-type: application/json' \
        -d '{"model":"nano-banana-pro"}' \
        -o downloads/nano-banana-pro-$(date +%Y%m%d).csv

Note: download-prompts.py in downloads/ automates all the above
      (with proxy + cookie auth), but is NOT committed to git.
      Set HTTP_PROXY env var or prepend --proxy to curl if needed.

Usage:
    python scripts/convert_ideas.py
"""

import csv
import json
import os
from urllib.parse import urlparse

NEXRA_IMAGE_PREFIX = "https://raw.githubusercontent.com/NeXra-AI/awesome-ai-image-prompts/refs/heads/main/"
OUTPUT = "docs/ideas.json"
CSV_DIR = "downloads"
KEEP_LANGS = {"en", "zh"}


def log(msg):
    print(f"  {msg}")


def read_nexra() -> list:
    """Read and convert the local downloads/prompts.json (already downloaded)."""
    path = os.path.join(CSV_DIR, "prompts.json")
    if not os.path.exists(path):
        log(f"  (not found: {path})")
        return []

    with open(path, encoding="utf-8") as f:
        raw = json.load(f)
    log(f"  NeXra: {len(raw)} entries")

    result = []
    for r in raw:
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

                has_zh = any("\u4e00" <= c <= "\u9fff" for c in content)
                lang = "zh" if has_zh else "en"

                author = ""
                try:
                    author = json.loads(r.get("author", "{}")).get("name", "")
                except json.JSONDecodeError:
                    author = r.get("author", "")

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
    seen_urls = set()
    seen_images = set()
    seen_content = set()
    merged = []

    def is_dup(entry: dict) -> bool:
        keys = []

        url = entry.get("source_url", "")
        if url:
            keys.append(("url", url))

        imgs = entry.get("image_urls", [])
        if imgs:
            for u in imgs:
                keys.append(("img", os.path.basename(urlparse(u).path)))

        if not keys:
            keys.append(("content", entry.get("title", "") + "|" + entry.get("prompt", "")))

        for kind, value in keys:
            if kind == "url" and value in seen_urls:
                return True
            if kind == "img" and value in seen_images:
                return True
            if kind == "content" and value in seen_content:
                return True

        for kind, value in keys:
            if kind == "url":
                seen_urls.add(value)
            elif kind == "img":
                seen_images.add(value)
            elif kind == "content":
                seen_content.add(value)

        return False

    def sort_key(entry: dict) -> tuple:
        url = entry.get("source_url", "") or ""
        return (
            url,
            entry.get("lang", ""),
            entry.get("title", ""),
            entry.get("prompt", "")[:100],
        )

    for source in [existing] + list(sources):
        for entry in source:
            if is_dup(entry):
                continue
            merged.append(entry)

    merged.sort(key=sort_key)
    return merged


def has_source_data() -> bool:
    """Return True if any source file exists (prompts.json or CSVs)."""
    prompts = os.path.join(CSV_DIR, "prompts.json")
    if os.path.exists(prompts):
        return True
    if not os.path.isdir(CSV_DIR):
        return False
    return any(f.endswith(".csv") for f in os.listdir(CSV_DIR))


def main():
    if not has_source_data():
        log(f"No source files found in {CSV_DIR}/ (prompts.json or *.csv)")
        return

    # Load existing entries if any
    existing = []
    if os.path.exists(OUTPUT):
        with open(OUTPUT, encoding="utf-8") as f:
            existing = json.load(f)
        log(f"Existing: {len(existing)} entries")

    nexra = read_nexra()
    csvs = read_csvs()

    merged = merge(existing, nexra, csvs)
    log(f"Merged: {len(merged)} entries (existing {len(existing)} + nexra {len(nexra)} + csv {len(csvs)})")

    with open(OUTPUT, "w", encoding="utf-8") as f:
        json.dump(merged, f, ensure_ascii=False, indent=2)
    log(f"Saved to {OUTPUT}")


if __name__ == "__main__":
    main()
