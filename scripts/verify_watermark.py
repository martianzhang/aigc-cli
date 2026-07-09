#!/usr/bin/env python3
"""
Watermark Removal Verification Script

Verifies that watermark removal was effective by comparing the original
and cleaned images pixel-by-pixel in the watermark region.

Usage:
    python scripts/verify_watermark.py <original> <clean> [--wm-x X] [--wm-y Y] [--wm-w W] [--wm-h H] [--producer NAME]

Arguments:
    original        Path to the original watermarked image
    clean           Path to the cleaned image (output of `aigc-cli detect --remove-watermark`)

Options:
    --wm-x X        Watermark X position (pixel). If omitted, auto-detected.
    --wm-y Y        Watermark Y position (pixel). If omitted, auto-detected.
    --wm-w W        Watermark width (pixel). If omitted, auto-detected.
    --wm-h H        Watermark height (pixel). If omitted, auto-detected.
    --producer NAME Watermark producer: "doubao", "jimeng", or "gemini".
                    Used for auto-positioning when --wm-x etc. are omitted.
    --threshold N   Pixel diff threshold (default: 5). Pixels with absolute
                    diff > N are counted as "changed".
    --crop          Save cropped watermark region as <clean>_wm_crop.png for visual inspection.

Examples:
    # Basic: auto-detect watermark position
    python scripts/verify_watermark.py image.png image_clean.png

    # Specify producer for auto-positioning
    python scripts/verify_watermark.py image.png image_clean.png --producer doubao

    # Manual watermark position (for non-standard images)
    python scripts/verify_watermark.py image.png image_clean.png --wm-x 1686 --wm-y 1931 --wm-w 335 --wm-h 83

    # With crop output for visual check
    python scripts/verify_watermark.py image.png image_clean.png --producer jimeng --crop

Exit codes:
    0 - Watermark fully removed (0 residual pixels)
    1 - Watermark partially removed (some residual pixels remain)
    2 - Error (file not found, invalid arguments, etc.)
"""
import sys
import os
import argparse
import math

try:
    from PIL import Image
    import numpy as np
except ImportError:
    print("Error: This script requires Pillow and NumPy.", file=sys.stderr)
    print("Install them with: pip install Pillow numpy", file=sys.stderr)
    sys.exit(2)


# Watermark geometry parameters (from internal/watermark/doubao.go and jimeng.go)
WM_PARAMS = {
    "doubao": {
        "native_w": 2048,
        "alpha_w": 335,
        "alpha_h": 83,
        "margin_r_frac": 0.0132,
        "margin_b_frac": 0.0166,
    },
    "jimeng": {
        "native_w": 2048,
        "alpha_w": 414,
        "alpha_h": 118,
        "margin_r_frac": 0.0288,
        "margin_b_frac": 0.0288,
    },
    "baidu": {
        "native_w": 1024,
        "alpha_w": 187,
        "alpha_h": 51,
        "margin_r_frac": 0.00293,
        "margin_b_frac": 0.00293,
    },
    "zhipu": {
        "native_w": 1024,
        "alpha_w": 234,
        "alpha_h": 60,
        "margin_r_frac": 0.0126953125,
        "margin_b_frac": 0.0078125,
    },
}


def calculate_wm_position(producer, w, h):
    """Calculate watermark position for Doubao/Jimeng using min(w,h) scaling."""
    if producer not in WM_PARAMS:
        return None
    p = WM_PARAMS[producer]
    shorter = min(w, h)
    scale = shorter / p["native_w"]
    sz_w = round(p["alpha_w"] * scale)
    sz_h = round(p["alpha_h"] * scale)
    margin_x = round(w * p["margin_r_frac"])
    margin_y = round(w * p["margin_b_frac"])
    x = w - margin_x - sz_w
    y = h - margin_y - sz_h
    return (x, y, sz_w, sz_h)


def find_changed_region(orig, clean, threshold=5):
    """Find the bounding box of changed pixels between original and clean."""
    diff = np.abs(orig.astype(int) - clean.astype(int))
    changed = diff.sum(axis=2) > threshold
    if changed.sum() == 0:
        return None
    ys, xs = np.where(changed)
    return (xs.min(), ys.min(), xs.max() - xs.min() + 1, ys.max() - ys.min() + 1, changed)


def verify(orig_path, clean_path, wm_pos=None, producer=None, threshold=5, crop=False):
    """Verify watermark removal quality."""
    orig = np.array(Image.open(orig_path).convert("RGB"))
    clean = np.array(Image.open(clean_path).convert("RGB"))
    h, w = orig.shape[:2]

    print(f"Image: {w}x{h}")
    print(f"Original: {orig_path}")
    print(f"Clean:    {clean_path}")
    print()

    # Determine watermark position
    if wm_pos:
        wx, wy, ww, wh = wm_pos
        print(f"Watermark position (manual): ({wx},{wy},{ww}x{wh})")
    elif producer:
        pos = calculate_wm_position(producer, w, h)
        if pos:
            wx, wy, ww, wh = pos
            print(f"Watermark position ({producer}): ({wx},{wy},{ww}x{wh})")
        else:
            print(f"Unknown producer: {producer}")
            sys.exit(2)
    else:
        # Auto-detect from changed region
        region = find_changed_region(orig, clean, threshold)
        if region:
            wx, wy, ww, wh, _ = region
            print(f"Watermark position (auto-detected): ({wx},{wy},{ww}x{wh})")
        else:
            print("No changes detected between original and clean!")
            sys.exit(1)

    # Ensure position is within bounds
    wx = max(0, min(wx, w - 1))
    wy = max(0, min(wy, h - 1))
    ww = min(ww, w - wx)
    wh = min(wh, h - wy)

    if ww < 1 or wh < 1:
        print("Invalid watermark region")
        sys.exit(2)

    # Extract watermark regions
    orig_r = orig[wy : wy + wh, wx : wx + ww]
    clean_r = clean[wy : wy + wh, wx : wx + ww]

    # Compute diff
    diff = np.abs(orig_r.astype(int) - clean_r.astype(int))
    changed_count = (diff.sum(axis=2) > threshold).sum()
    total = ww * wh

    print(f"\n--- Results ---")
    print(f"Region:     ({wx},{wy}) size {ww}x{wh} ({total} pixels)")
    print(f"Changed:    {changed_count} pixels ({changed_count/total*100:.1f}%)")
    print(f"Diff:       mean={diff.mean():.2f}, max={diff.max()}")

    # Check for residual watermark
    # On dark backgrounds: watermark = bright pixels
    # On light backgrounds: watermark = darker pixels
    orig_mean = orig_r.mean()
    clean_mean = clean_r.mean()

    if orig_mean < 100:
        # Dark background: check for bright remnants
        bright_b = (orig_r.max(axis=2) > 20).sum()
        bright_a = (clean_r.max(axis=2) > 20).sum()
        print(f"Bright(>20): {bright_b} -> {bright_a}")
        residual = bright_a
        metric = "bright"
    elif orig_mean > 200:
        # Light background: check std (watermark adds noise)
        orig_std = orig_r.std()
        clean_std = clean_r.std()
        print(f"Std:        {orig_std:.2f} -> {clean_std:.2f}")
        residual = 0 if clean_std <= orig_std else 1
        metric = "std"
    else:
        # Medium background: check for bright remnants
        bright_threshold = int(orig_mean * 1.5)
        bright_b = (orig_r.max(axis=2) > bright_threshold).sum()
        bright_a = (clean_r.max(axis=2) > bright_threshold).sum()
        print(f"Bright(>{bright_threshold}): {bright_b} -> {bright_a}")
        residual = bright_a
        metric = "bright"

    # Verdict
    print()
    if residual == 0:
        print(f"PASS: Watermark fully removed (0 residual {metric} pixels)")
        result = 0
    elif residual < total * 0.05:
        print(f"WARN: {residual} residual {metric} pixels ({residual/total*100:.1f}%)")
        result = 1
    else:
        print(f"FAIL: {residual} residual {metric} pixels ({residual/total*100:.1f}%)")
        result = 1

    # Optional: save crop
    if crop:
        pad = 20
        cx1 = max(0, wx - pad)
        cy1 = max(0, wy - pad)
        cx2 = min(w, wx + ww + pad)
        cy2 = min(h, wy + wh + pad)
        crop_path = clean_path.rsplit(".", 1)[0] + "_wm_crop.png"
        Image.open(clean_path).crop((cx1, cy1, cx2, cy2)).save(crop_path)
        print(f"\nCropped clean region saved to: {crop_path}")

    return result


def main():
    parser = argparse.ArgumentParser(
        description="Verify watermark removal quality by comparing original and cleaned images.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument("original", help="Path to the original watermarked image")
    parser.add_argument("clean", help="Path to the cleaned image")
    parser.add_argument("--wm-x", type=int, default=None, help="Watermark X position")
    parser.add_argument("--wm-y", type=int, default=None, help="Watermark Y position")
    parser.add_argument("--wm-w", type=int, default=None, help="Watermark width")
    parser.add_argument("--wm-h", type=int, default=None, help="Watermark height")
    parser.add_argument(
        "--producer",
        choices=["doubao", "jimeng", "gemini", "baidu", "zhipu"],
        default=None,
        help="Watermark producer for auto-positioning",
    )
    parser.add_argument("--threshold", type=int, default=5, help="Pixel diff threshold (default: 5)")
    parser.add_argument("--crop", action="store_true", help="Save cropped watermark region for visual inspection")

    args = parser.parse_args()

    if not os.path.exists(args.original):
        print(f"Error: File not found: {args.original}", file=sys.stderr)
        sys.exit(2)
    if not os.path.exists(args.clean):
        print(f"Error: File not found: {args.clean}", file=sys.stderr)
        sys.exit(2)

    wm_pos = None
    if all(v is not None for v in [args.wm_x, args.wm_y, args.wm_w, args.wm_h]):
        wm_pos = (args.wm_x, args.wm_y, args.wm_w, args.wm_h)

    result = verify(
        args.original,
        args.clean,
        wm_pos=wm_pos,
        producer=args.producer,
        threshold=args.threshold,
        crop=args.crop,
    )
    sys.exit(result)


if __name__ == "__main__":
    main()
