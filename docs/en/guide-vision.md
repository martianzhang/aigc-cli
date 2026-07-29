# Vision

Use `aigc-cli vision` to describe or analyze images using AI vision models.

## Usage

```bash
# Describe an image
aigc-cli vision describe photo.png

# With a specific prompt
aigc-cli vision describe photo.png --prompt "What colors are in this image?"
```

## Online Mode

Configure a provider to use online vision LLMs:

```yaml
defaults:
  vision:
    provider: "my-openrouter"
    model: "gpt-4o-vision"
```

## Offline Mode

Without a provider, vision falls back to EXIF metadata reading and basic image analysis.
