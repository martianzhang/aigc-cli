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

## Terminal Display

In iTerm2 and Sixel-capable terminals, `preview` renders the image inline instead of handing it to an external application:

- An image larger than the budget is scaled down to about 60% of the terminal width and height, so it never fills the whole window.
- An image that is already smaller than the budget is shown at its original size — previews are never enlarged.
- The aspect ratio is preserved and the image stays fully visible — nothing scrolls off.
- Sizing uses the terminal's real dimensions (columns × rows) at the time the command runs, so resize the window before running `preview`.
- When the terminal size cannot be detected (no TTY, or nothing reported), a conservative 80×24 grid is assumed so a large image is still bounded instead of being emitted at native size.
- On terminals without inline image support, the image is opened with the system default application instead.

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
