# 水印引擎指南

> 可见 AI 水印的检测、去除与添加。核心算法是逆 alpha 混合，所有水印共享同一套去除引擎。检测分两条路径：Gemini Sparkle（内置）和自定义文字水印（通过 `--learn-watermark` 学习）。

> ⚠️ **法律声明**
>
> 本软件首先是一个 **AIGC 检测与研究工具**。水印去除功能仅用于：
> 1. **验证检测算法的准确性** — 去除已知水印后确认检测结果
> 2. **合法图像修复** — 如修复个人旧照片、去除自己相机添加的日期水印
>
> **禁止用于：**
> - 未经授权去除他人版权图片的水印
> - 伪造或隐匿内容来源
> - 任何侵犯知识产权或违反适用法律法规的行为
>
> 用户应自行承担一切使用责任。软件作者不对用户的使用行为承担任何法律责任。

---

## 概述

| 水印 | 类型 | 说明 |
|---|---|---|
| **Gemini** (Google) | 内置 | ✦ Sparkle 图标，右下角固定像素边距。检测依赖尺寸目录，无法通过 `--learn-watermark` 学习 |
| **自定义水印** | 用户学习 | 通过 `--learn-watermark` 从黑底+灰底种子图自动求解 alpha map |

---

## 核心算法

所有引擎共用**逆 alpha 混合**（reverse alpha blending）：

```
watermarked = α × logo + (1 - α) × original
        ↓
original = (watermarked - α × logo) / (1 - α)
```

其中：
- `α` — alpha map 值 (0~1)，代表水印在该像素的透明度
- `logo` — 水印颜色（白色 = 255, 255, 255）
- `original` — 需要恢复的原始像素

---

## 学习自定义水印（`--learn-watermark`）

### 原理：两拍法

从两张同尺寸的纯色种子图中数学求解 alpha map：

```
黑底: pixel = 255 × α          → α = pixel / 255
灰底: pixel = 128 + 127 × α    → α = (pixel - 128) / 127
```

两声求平均，精度达到 NCC 0.999+。

### 步骤

1. 去 AI 平台生成两张纯色图（文生图，开启"添加水印"）：

| 文件名 | 颜色 | Prompt |
|---|---|---|
| `myai.black.png` | RGB(0,0,0) | "Generate a pure black image, RGB(0,0,0), no content. Aspect ratio 1:1." |
| `myai.gray.png` | RGB(128,128,128) | "Generate a pure gray image, RGB(128,128,128), no content. Aspect ratio 1:1." |

> 必须下载原始输出文件（原始 PNG/JPEG），不能截图。确保两张图分辨率一致。

2. 放到 `~/.config/aigc-cli/watermark/` 目录：

```
~/.config/aigc-cli/watermark/
├── myai.black.png
└── myai.gray.png
```

3. 学习：

```bash
aigc-cli detect --learn-watermark myai
```

输出 `~/.config/aigc-cli/watermark/myai.watermark.png`。这是一个自包含的 PNG 文件：
- 灰度像素 = alpha map（可用看图软件打开查看水印轮廓）
- PNG tEXt 元数据块 = 所有参数（native_width、margin 分数、threshold 等）

### 可选参数覆盖

```bash
aigc-cli detect --learn-watermark myai \
  --threshold 0.25 \
  --strategy inpaint
```

---

## 去水印（`--remove-watermark`）

```bash
# 用自定义水印（不需要 --confirm）
aigc-cli detect photo.png --remove-watermark --producer myai

# 用内置 Gemini（需要 --confirm）
aigc-cli detect photo.png --remove-watermark --producer gemini --confirm

# 自动检测（不指定 producer，会扫所有已加载的）
aigc-cli detect photo.png --remove-watermark --confirm
```

### 流程

```
1. 确定 producer
   ├─ --producer 手动指定
   ├─ TC260 ContentProducer 自动匹配
   └─ 通用检测（取最高置信度）

2. 检测水印位置（同检测流程）

3. 去除
   ├─ 多档 alpha gain 尝试（0.6-1.3）
   ├─ 位置精调（亚像素 / ±3px）
   ├─ 逆 alpha 混合
   ├─ 过减保护检查
   └─ 边缘清理（blendEdge / progressive inpaint）

4. 重编码保存（JPEG Q95 / PNG）
```

---

## 加水印（`--add-watermark`）

> ⚠️ 仅用于验证去水印算法的正确性，不注入任何元数据。

```bash
# 用内置 Gemini 样式
aigc-cli detect photo.png --add-watermark --producer gemini

# 用自定义文字
aigc-cli detect photo.png --add-watermark --producer "MyWatermark"
```

---

## 水印目录

所有自定义水印配置存放在 `~/.config/aigc-cli/watermark/`：

```
~/.config/aigc-cli/watermark/
├── myai.watermark.png    # 学习好的水印配置
├── myai.black.png        # 黑底种子图（可选，供日后重新学习）
├── myai.gray.png         # 灰底种子图（可选）
└── ...
```

每次 `detect` 运行时，自动加载目录下所有 `*.watermark.png` 文件。

---

## 参考项目

- **[gemini-watermark-remover](https://github.com/GargantuaX/gemini-watermark-remover)** — Gemini Sparkle 水印的 alpha map 数据和尺寸目录
- **[remove-ai-watermarks](https://github.com/wiltodelta/remove-ai-watermarks)** — 文字水印的 alpha map 资产和两拍法提取算法

---

## 局限性

- 逆 alpha 混合在纯色/渐变背景上效果最好，复杂纹理区域可能有轻微残影
- 仅支持叠加型可见水印（白/灰半透明覆盖），不支持植入型/隐写水印
- Gemini Sparkle 无法通过 `--learn-watermark` 学习（检测依赖硬编码尺寸目录）
- `--learn-watermark` 假设水印为白色（255,255,255），非白色水印需手动调整