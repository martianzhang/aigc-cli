#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
去水印效果可视化诊断工具

比较原图和去水印后的 clean 图，生成并排对比图 + 差异热力图，并标注已知水印的预期位置。

用法:
    python scripts/check_watermark.py <原图>
    python scripts/check_watermark.py <原图> <clean图>
    python scripts/check_watermark.py <原图> --no-clean
    python scripts/check_watermark.py <原图> --report

示例:
    python scripts/check_watermark.py PixPin_2026-07-09_08-10-14.png
    python scripts/check_watermark.py 66381cab97065c4a1e1f0a6a1b678689.jpeg 66381cab97065c4a1e1f0a6a1b678689_clean.jpeg --report

输出:
    <原图名>_diff_compare.png  并排对比（原图 | clean | 热力图）
    <原图名>_diff_heatmap.png  单独的热力图
    <原图名>_wm_region.png     水印区域裁剪放大图

依赖:
    pip install Pillow numpy
"""

import sys
import os
import argparse
from pathlib import Path
from PIL import Image, ImageDraw, ImageFont
import numpy as np


# ── 已知水印类型的参数表 ──────────────────────────────
WATERMARK_CONFIGS = {
    "doubao-snap": {
        "size": (118, 58),
        "positions": lambda w, h: [
            (10, 10),
            (0, 0),
            (30, 30),
        ],
        "color": "#FF4444",
        "name": "Doubao Snap (AI 生成)",
    },
    "baidu": {
        "size": (139, 42),
        "positions": lambda w, h: [
            (w - 30 - 139, h - 15 - 42),
        ],
        "color": "#44FF44",
        "name": "Baidu (百度 AI生成)",
    },
    "doubao": {
        "size": None,
        "positions": lambda w, h: [
            (w - int(w * 0.25), h - int(h * 0.08)),
        ],
        "color": "#4444FF",
        "name": "Doubao (豆包AI生成)",
    },
    "zhipu": {
        # Native 234×60 at 1024px shorter side; scales with min(w,h).
        "size": None,
        "positions": lambda w, h: [
            (
                w - int(round(w * 0.0126953125)) - int(round(234 * min(w, h) / 1024)),
                h - int(round(w * 0.0078125)) - int(round(60 * min(w, h) / 1024)),
            ),
        ],
        "color": "#FF44FF",
        "name": "Zhipu Qingyan (智谱清言)",
    },
}


def load_image(path):
    img = Image.open(path)
    if img.mode != "RGB":
        img = img.convert("RGB")
    return img


def compute_diff(img1, img2):
    arr1 = np.array(img1, dtype=np.float32)
    arr2 = np.array(img2, dtype=np.float32)
    diff = np.abs(arr1 - arr2)
    diff_gray = np.mean(diff, axis=2)
    return diff, diff_gray


def create_heatmap(diff_gray, max_val=50):
    h, w = diff_gray.shape
    norm = np.clip(diff_gray / max_val, 0, 1)
    heatmap = np.zeros((h, w, 3), dtype=np.uint8)
    for y in range(h):
        for x in range(w):
            v = norm[y, x]
            if v < 0.25:
                r, g, b = 0, int(v * 4 * 255), 255
            elif v < 0.5:
                r, g, b = 0, 255, int((0.5 - v) * 4 * 255)
            elif v < 0.75:
                r, g, b = int((v - 0.5) * 4 * 255), 255, 0
            else:
                r, g, b = 255, int((1.0 - v) * 4 * 255), 0
            heatmap[y, x] = [r, g, b]
    return Image.fromarray(heatmap)


def draw_watermark_boxes(img):
    draw = ImageDraw.Draw(img)
    w, h = img.size
    for key, cfg in WATERMARK_CONFIGS.items():
        size = cfg["size"]
        for pos in cfg["positions"](w, h):
            x, y = pos
            if size is None:
                bw, bh = int(w * 0.25), int(h * 0.08)
                x = max(0, w - bw - 20)
                y = max(0, h - bh - 20)
            else:
                bw, bh = size
            if x < 0 or y < 0 or x + bw > w or y + bh > h:
                continue
            draw.rectangle([x, y, x + bw, y + bh], outline=cfg["color"], width=3)
            try:
                font = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", 14)
            except Exception:
                font = ImageFont.load_default()
            draw.text((x, max(0, y - 20)), cfg["name"], fill=cfg["color"], font=font)


def analyze_watermark_region(diff_gray, img_w, img_h):
    results = []
    for key, cfg in WATERMARK_CONFIGS.items():
        size = cfg["size"]
        for pos in cfg["positions"](img_w, img_h):
            x, y = pos
            if size is None:
                bw, bh = int(img_w * 0.25), int(img_h * 0.08)
                x = max(0, img_w - bw - 20)
                y = max(0, img_h - bh - 20)
            else:
                bw, bh = size
            if x < 0 or y < 0 or x + bw > img_w or y + bh > img_h:
                continue
            region = diff_gray[y: y + bh, x: x + bw]
            if region.size == 0:
                continue
            results.append({
                "type": key,
                "x": x, "y": y,
                "w": bw, "h": bh,
                "mean_diff": float(np.mean(region)),
                "max_diff": float(np.max(region)),
                "nonzero_ratio": float(np.count_nonzero(region > 1) / region.size),
            })
    return results


def create_comparison(original, cleaned, diff_heatmap):
    w, h = original.size
    target_h = 600
    scale = target_h / h
    target_w = int(w * scale)

    orig_r = original.resize((target_w, target_h), Image.LANCZOS)
    clean_r = cleaned.resize((target_w, target_h), Image.LANCZOS)
    heat_r = diff_heatmap.resize((target_w, target_h), Image.NEAREST)

    draw_watermark_boxes(orig_r)
    draw_watermark_boxes(clean_r)
    draw_watermark_boxes(heat_r)

    total_w = target_w * 3 + 40
    canvas = Image.new("RGB", (total_w, target_h + 60), (30, 30, 30))
    canvas.paste(orig_r, (0, 30))
    canvas.paste(clean_r, (target_w + 20, 30))
    canvas.paste(heat_r, (target_w * 2 + 40, 30))

    draw = ImageDraw.Draw(canvas)
    try:
        font = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", 18)
    except Exception:
        font = ImageFont.load_default()
    for i, (label, color) in enumerate(
        zip(["Original", "Cleaned", "Diff Heatmap"], ["#FFFFFF", "#FFFFFF", "#FF6666"])
    ):
        x = i * (target_w + 20) + target_w // 2
        draw.text((x - 50, 5), label, fill=color, font=font)
    return canvas


def print_stats(stats):
    print("\n" + "=" * 60)
    print("[水印区域差异统计]")
    print("=" * 60)
    if not stats:
        print("[提示] 未匹配到任何已知水印类型的预期区域")
        return
    for s in stats:
        print(f"\n  {s['type']} @ ({s['x']}, {s['y']}) [{s['w']}x{s['h']}]")
        print(f"    平均差异: {s['mean_diff']:.2f}/255")
        print(f"    最大差异: {s['max_diff']:.2f}/255")
        print(f"    变化像素: {s['nonzero_ratio'] * 100:.1f}%")
        if s["mean_diff"] < 2:
            print(f"    [结论] 几乎无变化 -> 水印可能未被检测到")
        elif s["mean_diff"] < 10:
            print(f"    [结论] 变化很小 -> 移除不完整")
        elif s["mean_diff"] < 30:
            print(f"    [结论] 有变化 -> 水印区域被部分处理")
        else:
            print(f"    [结论] 变化显著 -> 水印被大幅修改")
    max_mean = max(s["mean_diff"] for s in stats) if stats else 0
    print(f"\n[总体评估]:")
    if max_mean < 2:
        print("   [失败] 水印区域几乎无变化，移除失败（检测可能未触发）")
    elif max_mean < 10:
        print("   [警告] 水印区域变化很小，移除不完整")
    else:
        print("   [通过] 水印区域有明显修改痕迹")


def crop_wm_region(orig, cleaned, diff_gray):
    """裁剪水印区域为单独放大图"""
    w, h = orig.size
    all_positions = []
    for key, cfg in WATERMARK_CONFIGS.items():
        size = cfg["size"]
        for pos in cfg["positions"](w, h):
            x, y = pos
            if size is None:
                bw, bh = int(w * 0.25), int(h * 0.08)
                x = max(0, w - bw - 20)
                y = max(0, h - bh - 20)
            else:
                bw, bh = size
            if x >= 0 and y >= 0 and x + bw <= w and y + bh <= h:
                all_positions.append((x, y, bw, bh))
    if not all_positions:
        return None
    x1 = min(p[0] for p in all_positions)
    y1 = min(p[1] for p in all_positions)
    x2 = max(p[0] + p[2] for p in all_positions)
    y2 = max(p[1] + p[3] for p in all_positions)
    pad = 20
    x1 = max(0, x1 - pad)
    y1 = max(0, y1 - pad)
    x2 = min(w, x2 + pad)
    y2 = min(h, y2 + pad)

    orig_crop = orig.crop((x1, y1, x2, y2))
    clean_crop = cleaned.crop((x1, y1, x2, y2))
    diff_crop_arr = diff_gray[y1:y2, x1:x2]
    diff_crop = Image.fromarray(np.clip(diff_crop_arr * 4, 0, 255).astype(np.uint8)).convert("RGB")

    cw, ch = x2 - x1, y2 - y1
    scale = 400 / cw
    nw, nh = 400, int(ch * scale)
    orig_crop = orig_crop.resize((nw, nh), Image.LANCZOS)
    clean_crop = clean_crop.resize((nw, nh), Image.LANCZOS)
    diff_crop = diff_crop.resize((nw, nh), Image.NEAREST)

    canvas = Image.new("RGB", (nw * 3 + 20, nh + 30), (30, 30, 30))
    canvas.paste(orig_crop, (0, 25))
    canvas.paste(clean_crop, (nw + 10, 25))
    canvas.paste(diff_crop, (nw * 2 + 20, 25))
    draw = ImageDraw.Draw(canvas)
    try:
        font = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", 14)
    except Exception:
        font = ImageFont.load_default()
    for i, label in enumerate(["Original", "Cleaned", "Diff"]):
        x = i * (nw + 10) + nw // 2
        draw.text((x - 30, 3), label, fill="#CCCCCC", font=font)
    return canvas


def main():
    parser = argparse.ArgumentParser(
        description="去水印效果可视化诊断工具",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument("original", help="原图路径")
    parser.add_argument("cleaned", nargs="?", default=None, help="clean 图路径（默认自动查找 *_clean.*）")
    parser.add_argument("--no-clean", action="store_true", help="无 clean 图，只显示原图和预期水印位置")
    parser.add_argument("--report", action="store_true", help="输出详细报告")
    parser.add_argument("--output", "-o", default=None, help="输出路径（默认 <原图名>_diff_compare.png）")
    args = parser.parse_args()

    if not os.path.exists(args.original):
        print(f"[错误] 文件不存在: {args.original}")
        sys.exit(1)

    print(f"[原图] {args.original}")
    orig = load_image(args.original)
    w, h = orig.size
    print(f"[尺寸] {w}x{h}")

    clean_path = args.cleaned
    if not clean_path and not args.no_clean:
        p = Path(args.original)
        for suffix in [p.suffix, ".png", ".jpg", ".jpeg"]:
            candidate = p.parent / (p.stem + "_clean" + suffix)
            if candidate.exists():
                clean_path = str(candidate)
                break

    if clean_path and os.path.exists(clean_path):
        print(f"[Clean] {clean_path}")
        clean = load_image(clean_path)
        if clean.size != (w, h):
            print(f"[警告] 尺寸不一致，缩放 clean 图匹配原图")
            clean = clean.resize((w, h), Image.LANCZOS)
        diff, diff_gray = compute_diff(orig, clean)
        if args.report:
            print(f"[全局平均差异] {np.mean(diff_gray):.2f}/255")
            print(f"[全局最大差异] {np.max(diff_gray):.2f}/255")
        stats = analyze_watermark_region(diff_gray, w, h)
        if args.report:
            print_stats(stats)
        heatmap = create_heatmap(diff_gray)
        comparison = create_comparison(orig, clean, heatmap)
        out_path = args.output or (Path(args.original).stem + "_diff_compare.png")
        comparison.save(out_path, quality=95)
        print(f"[输出] 对比图: {out_path}")
        heat_path = Path(args.original).stem + "_diff_heatmap.png"
        heatmap.save(heat_path)
        print(f"[输出] 热力图: {heat_path}")
        wm_crop = crop_wm_region(orig, clean, diff_gray)
        if wm_crop:
            wm_path = Path(args.original).stem + "_wm_region.png"
            wm_crop.save(wm_path)
            print(f"[输出] 水印区域: {wm_path}")

    elif args.no_clean:
        canvas = orig.copy()
        draw_watermark_boxes(canvas)
        out_path = args.output or (Path(args.original).stem + "_wm_boxes.png")
        canvas.save(out_path)
        print(f"[输出] 水印位置标注: {out_path}")
        print("[提示] 使用 --no-clean 模式，仅标注预期水印位置")
    else:
        print("[错误] 未找到 clean 图")
        print("   请指定 clean 图路径，或使用 --no-clean 只看水印位置标注")
        sys.exit(1)


if __name__ == "__main__":
    main()
