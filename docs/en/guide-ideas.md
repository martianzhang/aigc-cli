# Prompt Ideas

Use `aigc-cli ideas` (alias `idea`) to search the built-in prompt idea library. All search is offline (BM25, CJK-aware, n-gram, RRF) — no API call needed.

## Usage

```bash
# Random prompt idea
aigc-cli ideas

# Search by keyword
aigc-cli ideas "cat"
aigc-cli ideas "cyberpunk city"

# Search in Chinese
aigc-cli ideas "猫"
```

The idea library contains 10,000+ prompts covering various styles, subjects, and artistic techniques.
