# Preview

Use `aigc-cli preview` (alias `pr`) to view images, inspect metadata, and generate descriptions.

## Usage

```bash
# View an image in terminal
aigc-cli preview photo.png

# Show detailed metadata
aigc-cli preview photo.png --detail

# Generate a description
aigc-cli preview photo.png --describe
```

## Metadata Display

`--detail` shows EXIF metadata including:
- Camera make/model
- Date taken
- GPS coordinates
- Image dimensions
- File size
- Color profile

## Description

`--describe` uses AI vision (if configured) or EXIF data to generate an image description.
