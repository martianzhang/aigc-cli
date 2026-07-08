#!/usr/bin/env python3
"""
Alpha Map to Go Source Generator

Converts a watermark alpha map PNG image into a Go source file with a
float64 array suitable for embedding in the watermark package.

Usage:
    python scripts/generate_alpha_go.py <input.png> <varname> [--pkg watermark] [--output output.go]

Arguments:
    input.png       Path to the alpha map PNG image (grayscale or RGB).
    varname         Go variable name (e.g., "myModelAlphaRaw").

Options:
    --pkg NAME      Go package name (default: "watermark").
    --output FILE   Output Go source file. Default: <varname>_data.go
    --comment TEXT  Additional comment text to include in the file header.
    --values-per N  Number of float64 values per line (default: 16).

Examples:
    # Basic usage
    python scripts/generate_alpha_go.py model_alpha.png modelAlphaRaw

    # Custom package and output file
    python scripts/generate_alpha_go.py model_alpha.png modelAlphaRaw \\
        --pkg watermark --output internal/watermark/model_alpha.go

    # With description comment
    python scripts/generate_alpha_go.py alpha.png myAlphaRaw \\
        --comment "MyModel visible watermark, 200x50, captured at 1024px"

Input format:
    - Grayscale (mode L): pixel value / 255 = alpha
    - RGB (mode RGB): max(R,G,B) / 255 = alpha
    - RGBA (mode RGBA): max(R,G,B) / 255 = alpha (ignores alpha channel)
"""
import sys
import os
import re
import argparse

try:
    from PIL import Image
    import numpy as np
except ImportError:
    print("Error: This script requires Pillow and NumPy.", file=sys.stderr)
    print("Install them with: pip install Pillow numpy", file=sys.stderr)
    sys.exit(1)


def png_to_float64_array(input_path):
    """Read a PNG and return (width, height, flat_float64_array) with values in [0,1]."""
    img = Image.open(input_path).convert("RGB")
    w, h = img.size
    arr = np.array(img, dtype=np.float64)
    # Take max of RGB channels as alpha proxy
    alpha = arr.max(axis=2) / 255.0
    return w, h, alpha.flatten()


def generate_go_source(varname, w, h, data, pkg="watermark", comment=None):
    """Generate Go source code as a string."""
    lines = []
    lines.append(f"package {pkg}")
    lines.append("")

    if comment:
        lines.append(f"// {varname} is a {w}x{h} alpha map - {comment}.")
    else:
        lines.append(f"// {varname} is a {w}x{h} alpha map.")
    lines.append(f"// Values are float64 in [0,1].")
    lines.append(f"var {varname} = []float64{{")

    # Write values with n per line
    n_per_line = 16
    for i in range(0, len(data), n_per_line):
        chunk = data[i : i + n_per_line]
        vals = ", ".join(f"{v:.10f}" for v in chunk)
        lines.append(f"\t{vals},")

    lines.append("}")
    return "\n".join(lines) + "\n"


def infer_output_path(varname):
    """Infer output file path from variable name."""
    # Convert CamelCase to snake_case and append _data.go
    # e.g., "modelAlphaRaw" → "model_alpha_raw_data.go"
    # But simpler: just varname lowercase without Raw suffix + _data.go
    name = varname
    if name.endswith("Raw"):
        name = name[:-3]
    return f"{name.lower()}_alpha_data.go"


def main():
    parser = argparse.ArgumentParser(
        description="Convert an alpha map PNG to a Go float64 source file.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument("input", help="Path to the alpha map PNG image")
    parser.add_argument("varname", help="Go variable name (e.g., 'myModelAlphaRaw')")
    parser.add_argument("--pkg", default="watermark", help="Go package name (default: watermark)")
    parser.add_argument("--output", default=None, help="Output Go source file path")
    parser.add_argument("--comment", default=None, help="Additional comment text")
    parser.add_argument("--values-per", type=int, default=16, help="Float64 values per line (default: 16)")

    args = parser.parse_args()

    if not os.path.exists(args.input):
        print(f"Error: File not found: {args.input}", file=sys.stderr)
        sys.exit(1)

    w, h, data = png_to_float64_array(args.input)
    print(f"Image: {args.input}")
    print(f"  Dimensions: {w}x{h}")
    print(f"  Pixels: {len(data)}")
    print(f"  Alpha range: [{data.min():.4f}, {data.max():.4f}]")
    print(f"  Alpha mean: {data.mean():.4f}")

    def sanitize_comment(text):
        # Remove or replace non-ASCII characters for Go source compatibility
        if not text:
            return text
        text = text.replace("\u2014", "--").replace("\u2013", "--")
        text = text.replace("\u2018", "'").replace("\u2019", "'")
        text = text.replace("\u201c", '"').replace("\u201d", '"')
        text = text.replace("\u2026", "...")
        return re.sub(r'[^\x20-\x7E]', '', text)

    src = generate_go_source(args.varname, w, h, data, args.pkg, sanitize_comment(args.comment))

    output_path = args.output or infer_output_path(args.varname)
    with open(output_path, "w", encoding="utf-8") as f:
        f.write(src)

    print(f"\nGenerated: {output_path}")
    print(f"  Package: {args.pkg}")
    print(f"  Variable: {args.varname}")
    print(f"  Lines: {src.count(chr(10))}")


if __name__ == "__main__":
    main()
