#!/usr/bin/env python3
"""
Visible Watermark Alpha Solver

Extracts a pixel-exact alpha map from two controlled captures of a visible
watermark: one on PURE BLACK background and one on MEDIUM GRAY (~128) background.

This is the "two-capture method" — the gold standard for extracting alpha maps
of embedded watermarks (Doubao "豆包AI生成", Jimeng "即梦AI", etc.).

How it works (reference project's approach):
  1. Locate the watermark on the BLACK capture (bright pixels on black)
  2. Fit a per-channel CUBIC surface over the GRAY capture's non-watermark pixels
     to model the background gradient (a cubic handles gentle gradients without
     bleeding glyph values)
  3. Solve alpha per pixel:  a = (I - B) / (255 - B)
     where I = observed pixel on gray, B = fitted background
  4. Crop to the glyph body with a small halo

Usage:
    python scripts/visible_alpha_solve.py <name> <black.png> <gray.png> [--corner br|bl]

    <name>     Producer name, e.g. "baidu", "kling", "yuanbao"
    <black.png>  Watermark on pure black background (download, NOT screenshot!)
    <gray.png>   Watermark on pure gray ~128 background (download, NOT screenshot!)

Options:
    --corner      Watermark corner: "br" (bottom-right, default), "bl" (bottom-left)
    --width NATIVE  Native/full resolution width for geometry scaling (default: auto from black capture)
    --output DIR  Output directory for alpha PNG + Go source (default: scripts/assets/)
    --pkg NAME    Go package name for generated source (default: watermark)

Example:
    # Extract baidu watermark alpha map from captures
    python scripts/visible_alpha_solve.py baidu baidu_black.png baidu_gray.png

    # For a watermark in the bottom-left corner (e.g. Samsung)
    python scripts/visible_alpha_solve.py samsung samsung_black.png samsung_gray.png --corner bl

Output:
    - scripts/assets/<name>_alpha.png      (the extracted alpha map PNG)
    - scripts/assets/<name>_alpha_data.go  (Go source file, ready to register)

    Also prints the geometry constants you need to fill into Register(Config{...}).

Requires:
    pip install Pillow numpy
"""

import sys
import os
import argparse
from pathlib import Path

import numpy as np
from PIL import Image


# ── Constants ─────────────────────────────────────────────────────────
_CUBIC_BG_PAD = 30        # px of background margin around the mark
_GLYPH_BODY = 0.08        # alpha above this = glyph body (for bbox)
_MIN_PART_AREA = 25       # drop connected blobs smaller than this
_HALO_PAD = 7             # keep halo around glyph in saved asset


# ── Morphological operations (pure numpy, no scipy/cv2 needed) ────────

def _dilate_binary(mask: np.ndarray, k: int = 9) -> np.ndarray:
    """Binary dilation with square kernel of side k (must be odd)."""
    import math
    pad = k // 2
    out = np.zeros_like(mask)
    for dy in range(-pad, pad + 1):
        for dx in range(-pad, pad + 1):
            shifted = np.roll(np.roll(mask, dy, axis=0), dx, axis=1)
            if dy < 0: shifted[dy:, :] = False
            elif dy > 0: shifted[:dy, :] = False
            if dx < 0: shifted[:, dx:] = False
            elif dx > 0: shifted[:, :dx] = False
            out |= shifted
    return out


def _erode_binary(mask: np.ndarray, k: int = 9) -> np.ndarray:
    """Binary erosion with square kernel of side k (must be odd)."""
    import math
    pad = k // 2
    out = np.ones_like(mask)
    for dy in range(-pad, pad + 1):
        for dx in range(-pad, pad + 1):
            shifted = np.roll(np.roll(mask, dy, axis=0), dx, axis=1)
            if dy < 0: shifted[dy:, :] = True
            elif dy > 0: shifted[:dy, :] = True
            if dx < 0: shifted[:, dx:] = True
            elif dx > 0: shifted[:, :dx] = True
            out &= shifted
    return out


def _union_bbox(mask: np.ndarray, min_area: int = 25) -> tuple[int, int, int, int]:
    """Bounding box (x0, x1, y0, y1) of mask's non-zero pixels.
    
    On a clean black background the watermark is the only significant bright region,
    so the global bounding box of all pixels above threshold is sufficient.
    """
    ys, xs = np.where(mask)
    if len(xs) < min_area:
        raise ValueError(f"no watermark found (only {len(xs)} pixels above threshold)")
    return int(xs.min()), int(xs.max() + 1), int(ys.min()), int(ys.max() + 1)


def _locate_on_black(black: np.ndarray, corner: str) -> tuple[int, int, int, int]:
    """Bounding box of the bright watermark on pure black, in the given corner."""
    h, w = black.shape[:2]
    lum = black.mean(axis=2)
    bright = lum > 20  # comfortably above ~5-15 noise on near-black
    bright[: h * 3 // 4, :] = False  # bottom quarter only
    if corner == "bl":
        bright[:, w // 2 :] = False   # left half only
    else:
        bright[:, : w * 3 // 4] = False  # right quarter only
    return _union_bbox(bright)


def _cubic_background(crop: np.ndarray, glyph: np.ndarray) -> np.ndarray:
    """Per-channel cubic surface fit over non-glyph pixels."""
    h, w = crop.shape[:2]
    yy, xx = np.mgrid[0:h, 0:w].astype(np.float64)
    yy /= h
    xx /= w
    terms = [
        np.ones_like(xx), xx, yy,
        xx * xx, xx * yy, yy * yy,
        xx ** 3, xx * xx * yy, xx * yy * yy, yy ** 3,
    ]
    basis = np.stack(terms, axis=-1).reshape(-1, len(terms))
    keep = (~glyph).reshape(-1)
    out = np.zeros_like(crop, dtype=np.float64)
    for ch in range(3):
        values = crop[..., ch].reshape(-1).astype(np.float64)
        coef, *_ = np.linalg.lstsq(basis[keep], values[keep], rcond=None)
        out[..., ch] = (basis @ coef).reshape(h, w)
    return out


def solve_alpha(
    black: np.ndarray, gray: np.ndarray, corner: str = "br", native_width: int | None = None,
) -> tuple[np.ndarray, dict]:
    """
    Solve alpha map from black + gray captures.

    Returns (alpha_uint8_image, info_dict) where info_dict contains:
        - width, height: alpha map dimensions
        - abs_x0, abs_y0: absolute position in the capture
        - margin_*: geometry constants for Go Config registration
    """
    black_f = black.astype(np.float64)
    gray_f = gray.astype(np.float64)

    img_h, img_w = black_f.shape[:2]
    mx0, mx1, my0, my1 = _locate_on_black(black_f, corner)
    pad = _CUBIC_BG_PAD
    rx0 = max(0, mx0 - pad)
    rx1 = min(img_w, mx1 + pad)
    ry0 = max(0, my0 - pad)
    ry1 = min(img_h, my1 + pad)

    cg = gray_f[ry0:ry1, rx0:rx1]
    cb = black_f[ry0:ry1, rx0:rx1]

    # Glyph mask = dilated bright region on black capture
    glyph = _dilate_binary(cb.mean(axis=2) > 8, 9)

    # Fit cubic background on gray capture
    bg = _cubic_background(cg, glyph)

    # Solve alpha: a = (I - B) / (255 - B), averaged over channels
    denom = np.clip(255.0 - bg.mean(axis=2), 1e-3, None)
    alpha = np.clip((cg - bg).mean(axis=2) / denom, 0.0, 1.0)

    # Crop to glyph body + halo
    body = (alpha > _GLYPH_BODY).astype(np.uint8)
    bx, bex, by, bey = _union_bbox(body)
    cx0 = max(0, bx - _HALO_PAD)
    cy0 = max(0, by - _HALO_PAD)
    cx1 = min(alpha.shape[1], bex + _HALO_PAD)
    cy1 = min(alpha.shape[0], bey + _HALO_PAD)
    tight = alpha[cy0:cy1, cx0:cx1]
    aw, ah = tight.shape[1], tight.shape[0]

    # Absolute position in capture
    abs_x0 = rx0 + cx0
    abs_y0 = ry0 + cy0

    # Margins
    if corner == "bl":
        h_margin = abs_x0
    else:
        h_margin = img_w - (abs_x0 + aw)
    v_margin = img_h - (abs_y0 + ah)

    nw = native_width or img_w

    info = {
        "width": aw,
        "height": ah,
        "abs_x0": abs_x0,
        "abs_y0": abs_y0,
        "capture_width": img_w,
        "capture_height": img_h,
        "native_width": nw,
        "margin_x": h_margin,
        "margin_y": v_margin,
        "width_frac": aw / nw,
        "height_frac": ah / nw,
        "margin_x_frac": h_margin / nw,
        "margin_y_frac": v_margin / nw,
        "corner": corner,
        "alpha_max": float(tight.max()),
        "alpha_mean": float(tight[tight > 0].mean()) if (tight > 0).any() else 0.0,
    }

    out_img = (np.clip(tight, 0.0, 1.0) * 255.0).astype(np.uint8)
    return out_img, info


def generate_go_source(name: str, info: dict) -> str:
    """Generate Go source code for registering the watermark."""
    w = info["width"]
    h = info["height"]
    nw = info["native_width"]
    mx_frac = info["margin_x_frac"]
    my_frac = info["margin_y_frac"]
    w_frac = info["width_frac"]
    h_frac = info["height_frac"]
    corner = info["corner"]

    # Generate the variable name
    var_name = f"{name}AlphaRaw"

    return f"""package watermark

// Auto-generated by scripts/visible_alpha_solve.py
// {name} watermark, {w}x{h}, alpha max={info['alpha_max']:.3f}

func init() {{
	data := make([]float64, {w}*{h})
	for i := 0; i < {w}*{h}; i++ {{
		data[i] = {var_name}[i]
	}}
	am := NewAlphaMap({w}, {h}, data)

	Register(Config{{
		Type:            Type{name.title()},
		Name:            "{name}",
		AlphaMap:        am,
		DefaultSize:     {min(w, h)},
		DefaultMarginX:  {int(round(mx_frac * nw))},
		DefaultMarginY:  {int(round(my_frac * nw))},
		LogoColor:       [3]float64{{255, 255, 255}},
		DetectThreshold: 0.30,
		PositionResolver: func(w, h int) []Position {{
			shorter := w
			if h < shorter {{
				shorter = h
			}}
			scale := float64(shorter) / {nw}
			szW := int(round(float64({w}) * scale))
			szH := int(round(float64({h}) * scale))
			if szW < 20 || szH < 10 {{
				return nil
			}}
			marginX := int(round(float64(w) * {mx_frac}))
			marginY := int(round(float64(w) * {my_frac}))
			x := w - marginX - szW
			y := h - marginY - szH
			if x < 0 || y < 0 || x+szW > w || y+szH > h {{
				return nil
			}}
			return []Position{{{{X: x, Y: y, W: szW, H: szH}}}}
		}},
	}})
}}
"""


def main():
    parser = argparse.ArgumentParser(
        description="Extract pixel-exact watermark alpha map from black+gray captures",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument("name", help="Producer name, e.g. baidu, kling, yuanbao")
    parser.add_argument("black", help="Watermark on pure black background (PNG)")
    parser.add_argument("gray", help="Watermark on ~128 gray background (PNG)")
    parser.add_argument("--corner", choices=["br", "bl"], default="br",
                        help="Watermark corner: br=bottom-right (default), bl=bottom-left")
    parser.add_argument("--width", type=int, default=None,
                        help="Native/full resolution width (default: auto from black capture)")
    parser.add_argument("--output", "-o", type=str, default=None,
                        help="Output directory (default: scripts/assets/)")
    parser.add_argument("--pkg", default="watermark", help="Go package name")
    args = parser.parse_args()

    for p in [args.black, args.gray]:
        if not os.path.exists(p):
            print(f"Error: file not found: {p}")
            sys.exit(1)

    black = np.array(Image.open(args.black).convert("RGB"))
    gray = np.array(Image.open(args.gray).convert("RGB"))

    if black.shape != gray.shape:
        print(f"Warning: size mismatch! Black={black.shape[:2]}, Gray={gray.shape[:2]}")
        # Resize gray to match black
        gray_img = Image.fromarray(gray)
        gray_img = gray_img.resize((black.shape[1], black.shape[0]), Image.LANCZOS)
        gray = np.array(gray_img)

    alpha_img, info = solve_alpha(
        black, gray, corner=args.corner, native_width=args.width,
    )

    # Print geometry report
    print(f"\n{'='*60}")
    print(f"Alpha Map Report: {args.name}")
    print(f"{'='*60}")
    print(f"  Dimensions:    {info['width']}x{info['height']}")
    print(f"  Capture:       {info['capture_width']}x{info['capture_height']}")
    print(f"  Native width:  {info['native_width']}")
    print(f"  Alpha max:     {info['alpha_max']:.4f}")
    print(f"  Alpha mean(>0): {info['alpha_mean']:.4f}")
    print(f"\n  Geometry constants (fill into Register):")
    print(f"    WIDTH_FRAC        {info['width_frac']:.6f}")
    print(f"    HEIGHT_FRAC       {info['height_frac']:.6f}")
    print(f"    MARGIN_{'LEFT' if info['corner']=='bl' else 'RIGHT'}_FRAC  {info['margin_x_frac']:.6f}")
    print(f"    MARGIN_BOTTOM_FRAC {info['margin_y_frac']:.6f}")
    print(f"    Native width      {info['native_width']}")

    # Save alpha PNG
    output_dir = args.output
    if output_dir is None:
        output_dir = str(Path(__file__).parent / "assets")
    os.makedirs(output_dir, exist_ok=True)

    png_path = os.path.join(output_dir, f"{args.name}_alpha.png")
    Image.fromarray(alpha_img).save(png_path)
    print(f"\n  Saved: {png_path}")

    # Generate and save Go source (always, even if just for reference)
    go_src = generate_go_source(args.name, info)
    go_path = os.path.join(output_dir, f"{args.name}_alpha_data.go")
    with open(go_path, "w") as f:
        f.write(go_src)
    print(f"  Saved: {go_path}")

    # Also output the alpha_data.go variable data
    var_name = f"{args.name}AlphaRaw"
    flat = alpha_img.flatten().astype(np.float64) / 255.0
    print(f"\n  Go variable: {var_name}")
    print(f"  Array size: {len(flat)} floats ({info['width']}x{info['height']})")
    print(f"  Or use: aigc-cli detect --learn-watermark {args.name}")
    print(f"\n{'='*60}")


def round(x: float) -> int:
    return int(np.round(x))


def min(a: int, b: int) -> int:
    return a if a < b else b


if __name__ == "__main__":
    main()
