# AIGC 检测指南

`aigc-cli detect` 综合分析图片中的多种信号，输出 AI 生成置信度（AIGen rate）和直观的 emoji 标识。

---

## 快速开始

```bash
# 基础检测
aigc-cli detect image.png

# 检测并打开图片
aigc-cli detect --preview image.png

# JSON 输出
aigc-cli detect --json image.png

# 批量检测
aigc-cli detect *.png
```

---

## 检测结果解读

输出示例：

```
━━━ image.png ━━━
  Size:     186.19 KB
  Modified: 2026-07-02 10:09:15
  Format:   JPEG
  Dims:     1024 x 1024
  Watermark: C2PA Content Credentials          ← C2PA 元数据
    Vendor:   OpenAI Media Service
    Source:   AI Generated
  AI Detect:  🤖 99%  Confirmed AI-generated   ← 融合评分
    C2PA Content Credentials=100%              ← 各信号贡献
  Camera:   (none)
    (no camera EXIF found — likely not a real photograph)
```

### Emoji 含义

| Emoji | 含义 | 置信度 |
|---|---|---|
| 🟢 | 可能为人类创作 | 0-20% |
| 🟡 | 略微可疑 | 20-40% |
| 🟠 | 可能为 AI 生成 | 40-65% |
| 🔴 | 很可能为 AI 生成 | 65-90% |
| 🤖 | 确认 AI 生成（有元数据铁证） | 90-100% |

---

## 信号说明

`detect` 融合最多 **8 种信号**，加权得出最终评分。

### 铁证信号

| 信号 | 权重 | 触发条件 |
|---|---|---|
| **C2PA Content Credentials** | 🤖 铁证 | C2PA manifest 标注 `Source: AI Generated` |
| **TC260 AIGC Label** | 🤖 铁证 | 图片包含 GB 45438-2025 隐式标识 |

一旦检测到以上任一信号，直接输出 **🤖 99% Confirmed**，无需其他信号。

### 强信号

| 信号 | 权重 | 说明 |
|---|---|---|
| **SynthID 水印推断** | 🟠 高 | 通过 C2PA Vendor 字段推断（Google/OpenAI） |
| **Camera EXIF** | 🟢 强人类 | 有相机型号等信息=实拍，会大幅降低 AIGen 率 |
| **ONNX 模型** | 🟡 中 | ViT-Base 86M 参数 ML 模型，需先 `detect init` |
| **JPEG 量化表** | 🟡 中 | 检测量化表是否标准（非标准=AI 生成可能性高） |

### 弱信号

| 信号 | 权重 | 说明 |
|---|---|---|
| **SRM 噪声残差** | 🔵 低 | 5×5 高通滤波分析像素级周期性伪影 |
| **FFT 频谱分析** | 🔵 低 | 2D FFT 频域功率谱偏差（GAN 伪影检测） |
| **无 EXIF** | 🔵 弱 | 截图/AI 图常见，真实照片通常有 EXIF |

### 信号融合示例

```
🟠 52% Possibly AI-generated
  No Camera EXIF=55%; AI Model=73%; FFT Spectral=6%;
  Noise Residual=75%; JPEG Analysis=65%

🟢 17% Likely human-made
  Camera EXIF=10%; No Camera EXIF=55%; AI Model=3%; FFT Spectral=9%
  ↑ 有相机 EXIF + ONNX 模型低分 = 人类创作

🤖 99% Confirmed AI-generated
  C2PA Content Credentials=100%
  ↑ C2PA 铁证，直接锁定
```

---

## ONNX 模型检测（离线 ML 推理）

`detect` 支持通过 ONNX Runtime 运行 ViT 模型进行像素级 AI 检测。**纯 Go 实现，零 CGO 依赖**。

### 安装模型

```bash
# 下载大模型（ViT-Base 86M 参数，327MB，推荐）
aigc-cli detect init

# 下载小模型（distilled ViT 11.8M 参数，56MB）
aigc-cli detect init --model distilled-vit

# 强制重新下载
aigc-cli detect init --force
```

模型文件保存到 `~/.config/aigc-cli/models/`：
```
~/.config/aigc-cli/models/
├── onnxruntime.dll               ← ONNX Runtime 动态库（15MB）
├── model-vit-base.onnx           ← vit-base 模型（327MB，默认下载）
└── model-distilled-vit.onnx      ← distilled-vit 模型（56MB）
```

### 检测优先级

当两个模型都存在时，优先使用大模型：
```
model-vit-base.onnx → 有就用
   ↓ 没有
model-distilled-vit.onnx → 有就用
   ↓ 没有
ONNX 检测不可用
```

### 支持的平台

| 平台 | 运行时文件 |
|---|---|
| Windows x64 | `onnxruntime.dll` |
| Linux x64 | `libonnxruntime.so` |
| macOS arm64 | `libonnxruntime.dylib` |

---

## `--preview` 标志

检测完成后自动调用系统默认程序打开图片：

```bash
aigc-cli detect --preview image.png
# 终端输出检测结果，同时弹出系统看图软件
```

---

## `--json` 输出

结构化 JSON，适合脚本处理：

```bash
aigc-cli detect --json image.png
```

```json
{
  "path": "image.png",
  "size": 161170,
  "format": "PNG",
  "width": 2250,
  "height": 2279,
  "c2pa": {
    "present": true,
    "vendor": "OpenAI Media Service",
    "source": "AI Generated"
  },
  "ai_detect": {
    "ai_gen_rate": 0.99,
    "emoji": "🤖",
    "summary": "🤖 99% Confirmed AI-generated"
  }
}
```

---

## 信号技术细节

### FFT 频谱分析

对图片做 2D FFT（行-列法），计算径向平均功率谱，提取两个特征：

- **高频能量比**：空间频率后 30% 的能量占比。AI 生成图往往偏高或偏低。
- **log-log 斜率**：功率谱在频域的衰减斜率。自然图约 -2.0（1/f²），GAN 图常偏平坦。

使用 `gonum.org/v1/gonum/dsp/fourier`，纯 Go 实现。

### SRM 噪声残差

使用 Fridrich & Kodovský 2012 提出的 5×5 高通核（Spatial Rich Model）：
```
  -1   2  -2   2  -1
   2  -6   8  -6   2
  -2   8 -12   8  -2
   2  -6   8  -6   2
  -1   2  -2   2  -1
```
计算残差的 **标准差** 和 **峰度**。AI 图常出现过平滑（std 偏低）或周期性伪影（峰度偏高）。

### JPEG 量化表分析

扫描 JPEG DQT（Define Quantization Table）标记，与 ISO 标准量化表对比：

- **标准表** → 可能来自相机或标准编码器
- **非标准表** → 可能来自 AI 生成工具的自定义编码
- **多组量化表** → 可能经过二次压缩（真实照片编辑后保存）

---

---

## 可见水印去除（`--remove-watermark`）

从 AI 生成图片中检测并去除可见水印。支持以下平台：

| 平台 | 水印内容 | 布局 |
|---|---|---|
| **Gemini** (Google) | ✦ Sparkle 图标 | 右下角，固定像素边距 |
| **豆包** (ByteDance) | "豆包AI生成" 文字 | 右下角，按图片宽度等比缩放 |
| **即梦** (ByteDance) | "★ 即梦AI" 文字 | 右下角，按图片宽度等比缩放 |

### 用法

```bash
# 自动检测并去除（优先使用 TC260 ContentProducer 路由）
aigc-cli detect --remove-watermark image.png

# 手动指定 producer
aigc-cli detect --remove-watermark --producer doubao image.png
aigc-cli detect --remove-watermark --producer jimeng image.png
aigc-cli detect --remove-watermark --producer gemini image.png

# 去除后自动预览
aigc-cli detect --remove-watermark --preview image.png
```

### 路由逻辑

1. **TC260 元数据路由** — 如果图片的 TC260 `ContentProducer` 字段已知（如 `doubao`、`jimeng`），优先使用对应的位置解析器，以低阈值（0.08）在精确位置检测
2. **手动 `--producer` 覆盖** — 指定后跳过元数据检测，直接使用对应引擎
3. **通用 NCC 模板匹配** — 无元数据或无 producer 指定时，遍历所有注册配置，取最高置信度

### 技术方案

采用**逆 alpha 混合**（reverse alpha blending）：

```
original = (pixel - alpha × logo) / (1 - alpha)
```

- 多档 alpha gain 尝试（0.6–1.3）
- 亚像素精调（±0.5px，±2% 缩放）
- 边缘残余邻域修复
- 最多 4 轮迭代去除

**Gemini** 使用固定尺寸目录匹配（官方 Gemini 输出尺寸 → 已知水印大小 + 边距），加上近官方尺寸的投影推算。

**豆包 / 即梦** 使用分数定位（alpha map 尺寸和边距按图片宽度等比缩放），alpha map 在 2048px 基准宽度下提取。

### 参考项目

本实现参考了以下开源项目：

- **[gemini-watermark-remover](https://github.com/GargantuaX/gemini-watermark-remover)** — Gemini Sparkle 水印的 alpha map 数据（`embeddedAlphaMaps.js`）和尺寸目录（`geminiSizeCatalog.js`）
- **[remove-ai-watermarks](https://github.com/wiltodelta/remove-ai-watermarks)** — 豆包 "豆包AI生成" 和即梦 "★ 即梦AI" 文字水印的 alpha map 资产和分数定位参数

### 局限性

- 逆 alpha 混合在纯色/渐变背景上效果最好，复杂纹理区域可能有轻微残影
- 文字水印的 alpha map 在 2048px 基准宽度下训练，大幅偏离此尺寸的图片可能定位偏差
- 仅支持叠加型可见水印（白/灰半透明覆盖），不支持植入型水印

---

## 常见问题

### 需要联网吗？

基础检测（C2PA、TC260、SynthID、EXIF、FFT、噪声分析）**完全离线**，无需网络。

ONNX 模型检测需要先运行 `detect init` 下载模型文件（走一次网），之后也是离线运行。

### 需要 API Key 吗？

不需要。detect 命令完全独立于 API Key。

### 为什么有些 AI 图评分低？

三种原因：
1. **模型没见过这类 AI 图**——ONNX 模型训练数据有限，部分风格识别不准
2. **有损压缩抹掉了痕迹**——JPEG 压缩会破坏像素级特征，FFT 和噪声分析受影响
3. **没有 C2PA/TC260 元数据**——元数据是铁证，但没有时只能靠像素级信号

建议：打开 `--preview` 对照图片人工判断。

### 支持哪些图片格式？

PNG、JPEG、WebP、GIF、BMP。
