# 背景处理

`aigc-cli background` 基于 **RMBG 2.0**（BiRefNet 语义分割模型）去除或替换图片背景。支持任意背景类型——纯色、渐变、复杂场景均可。

> **核心算法**：RMBG 2.0（BRIA AI）ONNX 模型，纯 Go 本地推理，无需 API Key。

---

## 快速开始

```bash
# 首次使用：下载模型（~366MB）
aigc-cli background init

# 去除背景（主体变透明）
aigc-cli background input.png --remove

# 替换背景色
aigc-cli background input.png --replace "#FF0000"

# 替换为另一张图作为背景
aigc-cli background input.png --replace ./scenic_bg.jpg

# 裁剪到主体大小（不留多余透明区域）
aigc-cli background input.png --remove --autocrop

# 裁剪 + 四周留 30px 边距 + 强制 3:4 比例
aigc-cli background input.png --remove --ac --padding 30 --ar "3:4"

# 加上投影
aigc-cli background input.png --remove --ac --shadow --shadow-offset "6,6"
```

---

## 模型说明

| 项目 | 值 |
|---|---|
| 模型 | RMBG 2.0 (BiRefNet, Swin-V1 Large backbone) |
| 格式 | ONNX INT8 quantized (~366MB) |
| 来源 | [briaai/RMBG-2.0](https://huggingface.co/briaai/RMBG-2.0) |
| 许可证 | CC BY-NC 4.0（非商业） |
| 输入 | 1024×1024 RGB |
| 依赖 | ONNX Runtime（与 `detect` 命令共享） |

> **商业使用**：CC BY-NC 4.0 不允许商业场景。如需商用，请联系 [BRIA AI](https://www.bria.ai/) 获取授权。

---

## 命令说明

### 基本用法

```bash
aigc-cli background <图片路径...> [标志]
```

### 首次初始化

```bash
aigc-cli background init
```

从 HuggingFace 下载 RMBG 2.0 模型到 `~/.config/aigc-cli/models/model-rmbg-2.0.onnx`。ONNX Runtime 与 `detect init` 共享，如果已运行过 `aigc-cli detect init`，只需下载模型文件。

支持 `--force` 强制重新下载：

```bash
aigc-cli background init --force
```

> **下载耗时**：366MB 模型通过代理下载可能需要 1-5 分钟，请耐心等待。

### 去背（--remove）

```bash
aigc-cli background photo.jpg --remove
# 输出: photo_removebg.png（透明背景 PNG）
```

默认使用 RMBG AI 语义分割。支持批量处理：

```bash
aigc-cli background photo1.png photo2.png photo3.png --remove
```

### 替换背景（--replace, -r）

支持两种替换方式：

**颜色替换：**

```bash
aigc-cli background photo.png --replace "#FF0000"
aigc-cli background photo.png --replace "#00AAFF"
```

**图片替换：**

```bash
aigc-cli background photo.png --replace ./beach_bg.jpg
aigc-cli background photo.png --replace ./gradient_bg.png
```

### 调试遮罩（--mask-only）

输出灰度 alpha 遮罩（白=前景，黑=背景）：

```bash
aigc-cli background photo.png --mask-only
# 输出: photo_mask.png
```

### 自动裁剪（--autocrop / -c / --ac）

裁剪到主体边界框，去除多余的透明区域：

```bash
aigc-cli background photo.png --remove --autocrop
```

**组合使用：**

```bash
# 裁剪 + 四周各留 50px 边距
aigc-cli background photo.png --remove --ac --padding 50

# 裁剪 + 上下左右不同边距
aigc-cli background photo.png --remove --ac --padding "10,20,30,40"

# 裁剪 + 强制 1:1 比例
aigc-cli background photo.png --remove --ac --ar "1:1"

# 裁剪 + 16:9 比例 + 边距
aigc-cli background photo.png --remove --ac --padding 30 --ar "16:9"
```

### 投影（--shadow / -s）

在主体背后添加投影：

```bash
# 默认投影（右下偏移 4px，模糊 6px，40% 不透明度）
aigc-cli background photo.png --remove --shadow

# 自定义投影
aigc-cli background photo.png --remove --shadow \
  --shadow-offset "8,8"   `# 偏移` \
  --shadow-blur 12        `# 模糊半径` \
  --shadow-color "#000000" `# 颜色` \
  --shadow-opacity 50     `# 不透明度`
```

投影在 autocrop 之前计算，所以投影区域会被包含在裁剪范围内。

### 其他输出选项

```bash
# 指定输出目录
aigc-cli background photo.png --remove -o ./output/

# 输出 JSON 元数据
aigc-cli background photo.png --remove --json

# 处理后在系统预览中打开
aigc-cli background photo.png --remove --preview
```

---

## 标志完整列表

| 标志 | 简写 | 默认 | 说明 |
|---|---|---|---|
| `--remove` | `--rm` | false | 去除背景（输出透明 PNG） |
| `--replace` | `-r` | "" | 替换背景：颜色 `#RRGGBB` 或图片路径 |
| `--mask-only` | | false | 只输出灰度 alpha 遮罩 |
| `--autocrop` | `-c`, `--ac` | false | 裁剪到主体边界框 |
| `--padding` | | "" | 裁剪边距：`"20"` 或 `"10,20,30,40"` |
| `--aspect-ratio` | `--ar` | "" | 强制输出宽高比：`"1:1"`, `"16:9"` |
| `--shadow` | `-s` | false | 添加投影 |
| `--shadow-offset` | | `"4,4"` | 投影偏移 `"dx,dy"` |
| `--shadow-blur` | | 6 | 投影模糊半径 |
| `--shadow-color` | | `"#000000"` | 投影颜色 |
| `--shadow-opacity` | | 40 | 投影不透明度 0-100 |
| `--output` | `-o` | `.` | 输出目录 |
| `--preview` | `-p` | false | 在系统预览中打开 |
| `--json` | `-j` | false | JSON 格式输出 |

子命令：

| 子命令 | 说明 |
|---|---|
| `init` | 下载 RMBG 2.0 模型和 ONNX Runtime |

---

## 工作流程示例

### 1. 产品图去底 + 替换为白底

```bash
aigc-cli background product.jpg --replace "#FFFFFF" --autocrop --padding 20 --ar "1:1"
```

### 2. 人物头像去底 + 投影

```bash
aigc-cli background portrait.jpg --remove --ac --shadow --shadow-offset "3,3" --shadow-blur 4
```

### 3. 批量处理目录下所有 jpg

```bash
for f in *.jpg; do
  aigc-cli background "$f" --remove -o ./output/
done
```

### 4. 去底后在其他场景合成

```bash
# 先去掉背景
aigc-cli background subject.png --remove -o ./output/
# 然后用输出作为 --replace 的输入处理另一张图（或手动合成）
```

---

## 常见问题

### Q: 需要联网吗？

模型下载时需要联网（`background init`）。后续推理完全离线。

### Q: 支持哪些图片格式？

所有 Go `image.Decode` 支持的格式：PNG、JPEG、WebP、BMP、GIF。输出固定为 PNG（支持透明度）。

### Q: 和不带 AI 的旧版色度键控有什么区别？

| 维度 | 旧版 Chroma Key | RMBG 2.0（当前） |
|---|---|---|
| 适用范围 | 仅纯色/渐变背景 | 任意背景 |
| 算法 | CIELAB ΔE 色彩距离 | 语义分割（BiRefNet） |
| 精度 | 依赖背景色检测 | 像素级分割 |
| 模型依赖 | 无 | 366MB ONNX 模型 |
| 处理速度 | 亚秒级 | 数秒（GPU 可加速） |

### Q: 投影参数没效果？

确保同时指定了 `--shadow`（或 `-s`）和 `--remove`。投影是基于 alpha 遮罩在主体背后渲染的，所以必须在去背模式下使用。

### Q: Mac Apple Silicon 可以用 GPU 加速吗？

ONNX Runtime 支持 CoreML Execution Provider，会自动利用 Apple Neural Engine（ANE）。目前通过 CPU 推理已经很快，后续版本会默认启用 CoreML EP。

---

## 技术架构

```
用户输入图片
    │
    ▼
cmd/background.go: 解析标志 → 初始化 RMBG Detector
    │
    ▼
internal/rmbg: ONNX 推理
    ├── Preprocess: resize 1024² → ImageNet normalize
    ├── ONNX inference (pure-onnx)
    └── Postprocess: mask resize → apply alpha
    │
    ▼
internal/background:
    ├── shadow（可选）
    ├── autocrop（可选）
    └── composite（可选替换）
    │
    ▼
SavePNG → 输出文件
```
