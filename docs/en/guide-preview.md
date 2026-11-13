# Preview

Use `aigc-cli preview` (alias `pr`) to view images, Markdown and text files, inspect metadata, and generate descriptions.

## Usage

```bash
# View an image in terminal
aigc-cli preview photo.png

# Show detailed metadata
aigc-cli preview photo.png --detail

# Generate a description
aigc-cli preview photo.png --describe

# Preview a Markdown document (rendered in the terminal)
aigc-cli preview ocr_notes.md

# Preview plain text (paged automatically when long)
aigc-cli preview notes.txt
```

## Markdown Preview

`preview` reuses the chat TUI's Markdown renderer (same glamour styles, table handling and wrapping), so `.md` / `.markdown` files render with headings, lists, tables and code blocks instead of dumping the source:

- Rendering only happens when **stdout is a real terminal**. Piped or redirected output (`| grep`, `> notes.txt`, logs) stays byte-for-byte raw Markdown so it can be processed further.
- `--raw` forces raw source even in a terminal.
- Width follows the terminal columns, with a 20-column minimum for very narrow windows.
- `NO_COLOR` drops colors but keeps the layout.
- Examples:

```bash
# Rendered in the terminal
aigc-cli preview ocr_notes.md

# Render several documents in sequence
aigc-cli pr docs/*.md

# Keep the source (pipes usually do not need this)
aigc-cli preview notes.md --raw | grep TODO
```

## Markdown Images

On terminals with inline image support (iTerm2 / Kitty / Sixel), images referenced by the document are shown in place, with the alt text (the text in the square brackets; no caption when it is empty) below:

- Supports `![alt](path)`, `![alt](path "title")`, reference-style `![alt][id]`, and `<img src="...">`.
- Relative paths resolve against the Markdown file's directory; absolute paths, `file://`, `http(s)://` and `data:` targets also work.
- Remote and `data:` images are downloaded into a temporary file and deleted right after the preview.
- When the terminal does not support inline images, or `--no-inline-preview` / `AIGC_CLI_NO_INLINE_PREVIEW` is set, remote images are not downloaded and the reference stays as text.
- Images inside code blocks, inline code spans and links are left alone so the document structure is not broken.

## Paging

Markdown and plain text (`.txt` / `.text` / `.log`) taller than the terminal automatically open in a pager, like `bat`:

- `$PAGER` wins when set; otherwise `less -R` (colours preserved), then `more`.
- Arrow keys/`j`/`k` scroll, space pages, `q` quits. The text stays in the scrollback after quitting (`less` runs with `LESS=FRX`, no alternate screen).
- `--no-pager` (or `PAGER=cat`) prints directly; piped or redirected output is never paged.
- Markdown with inline images is not paged: pager redraws break image escape placement. Add `--no-inline-preview` to page such a document (its images stay as text references).

## Terminal Display

In iTerm2 and Sixel-capable terminals, `preview` renders the image inline instead of handing it to an external application:

- Inline rendering only happens when **stdout is a real terminal**. Piped, redirected, or captured output (`| tee`, log files, IDE/TUI terminals) would otherwise receive the escape sequence as literal text — the base64 blob you would see is the image itself.
- The payload is never the original file: an image larger than the budget is downscaled and re-encoded (JPEG, or PNG when the image has transparency), so a multi‑megabyte PNG previews from a few dozen KB.
- The embedded payload is capped at 512 KB. An image that still exceeds the cap (for example an animated GIF, which is sent verbatim so the animation survives) is opened with the system default application instead.
- The inline escape is skipped entirely when `AIGC_CLI_NO_INLINE_PREVIEW` is non-empty or `--no-inline-preview` is passed.
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
