# 水印引擎指南

> 可见 AI 水印的检测、去除与添加。本项目统一了两种架构不同的水印引擎（Gemini Sparkle / 文字水印），共用逆 alpha 混合核心算法，但检测与定位策略各自独立。

---

## 概述

支持以下可见 AI 水印：

| 平台 | 水印内容 | 类型 | Alpha Map | 布局 |
|---|---|---|---|---|
| **Gemini** (Google) | ✦ 星形 Sparkle | 图标（48-96px） | `gemini_bg_96.png` | 右下角，固定像素边距 |
| **豆包** (ByteDance) | "豆包AI生成" | 文字（335×83 @2048px） | `doubao_alpha.png` | 右下角，按图片短边缩放 |
| **即梦** (ByteDance) | "★ 即梦AI" | 文字（414×118 @2048px） | `jimeng_alpha.png` | 右下角，按图片短边缩放 |

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

## 引擎架构对比

本项目实现了两种架构完全不同的水印引擎。参考项目（`remove-ai-watermarks`）在 `_text_mark_engine.py` 中也明确指出：

> **Gemini stays a SEPARATE engine**: its multi-size fixed-slot sparkle model is genuinely different, not a tuned variant of this one.

| 维度 | Gemini (Sparkle) | 豆包/即梦 (文字) |
|---|---|---|
| **水印本质** | 半透明白色星形图标 | 中文文字叠加层 |
| **Alpha Map 尺寸** | 48×48 / 96×96（正方形） | 335×83 / 414×118（长方形） |
| **Alpha Map 来源** | 纯黑背景捕获图取 max(R,G,B)/255 | 黑+灰两色捕获图上做三次背景拟合（参考项目 `visible_alpha_solve.py`） |

### 检测方法

| | Gemini | 文字水印 |
|---|---|---|
| **搜索策略** | 动态搜索右下角 512×512 区域 | 固定几何定位 + NCC 对齐修正 |
| **模板匹配** | 灰度 NCC（`cv2.matchTemplate`）在多尺度（16-118px，步长 2）上滑窗 | 二值掩码（bright + low-sat + tophat）与 alpha 轮廓的 NCC 匹配 |
| **融合公式** | spatial×0.5 + gradient×0.3 + variance×0.2 | 同上（共用 `scoreCandidate`） |
| **尺寸处理** | 穷举全部 ~52 种尺寸 | PositionResolver 基于 `min(w,h)` 计算预期尺寸，对齐搜索 11 个 scale step（[0.6, 1.4]） |
| **位置确定** | Category 精确匹配 → 近官方投影 → 缩小搜索 → **每个种子 ±50px + 动态搜索** | 固定分数（`PositionResolver`）→ 粗搜 stride 4 + 精搜 stride 1 → **±60px / ±50% size** |
| **特殊处理** | Corner Promotion：小尺寸高保真 sparkle 的特殊召回 | 白色/浅色背景回退（binary mask 无法提取文字 → 直接用 PositionResolver 位置） |

### 去除方法

| | Gemini | 文字水印 |
|---|---|---|
| **Alpha 增益** | 自适应增益估计（`[1.0, 1.94]`）：根据 sparkle 核心亮度 vs 背景推算实际透明度，按比例缩放 alpha | 多组固定增益尝试（0.6/0.8/1.0/1.15/1.3），取 residual 最低者 |
| **位置精调** | 亚像素变形（`warpAlphaMap`）：±0.5px 位移，±2% 缩放 | 整数像素偏移：±3px 遍历 |
| **过减保护** | 双层：① numerator < 0 比例 > 5% → inpaint；② 预测核心 < 背景 - 25 级 → inpaint | 单层：预测核心 < 背景 - 25 级 → inpaint |
| **验证与修复** | verify-and-repair：去除完毕后重新检测，如果 sparkle 还在则 inpaint 替换 | 无 |
| **边缘清理** | `blendEdgeResidual`（IDW 混合 + alpha 梯度权重）+ 薄层 inpaint (`floor=0.01, dilate=3, radius=3`) | 渐进式边界生长 inpaint（`floor=0.03, dilate=7, radius=3`） |

### 误报处理

| | Gemini | 文字水印 |
|---|---|---|
| **FP 门槛** | 低置信度 + (低 core-ring 亮度差 OR 低梯度 NCC) → 降权到 0.30 | 仅 NCC 阈值（0.30-0.45） |
| **原因** | 星形+光晕轮廓在自然图像中可能偶然匹配 | "豆包AI生成" 等中文字串在自然图像中几乎不可能偶然出现 |

---

## 检测流程

```
输入图片
  │
  ├─ 1. 元数据检测（C2PA / TC260 / SynthID / EXIF）
  │      └─ 如果 C2PA "AI Generated" 或 TC260 已存在 → 跳过水印检测（已铁证）
  │
  ├─ 2. 可见水印检测（每次 detect 运行）
  │      ├─ Gemini: resolveWatermarkConfigs() → NCC 评分 → coarse+fine 搜索
  │      ├─ Doubao: extractBinaryMask() → alignByNCC() → 多尺度 NCC 对齐
  │      └─ Jimeng: 同上
  │      └─ 检测到水印 → forensic.Analyze 接收 WatermarkPresent → ironclad 信号
  │
  ├─ 3. 其他分析（ONNX / FFT / 噪声 / JPEG）
  │
  └─ 4. forensic.Analyze 融合所有信号 → AIGenRate + emoji
```

检测到可见水印时，作为 **ironclad** 级别信号（同 C2PA / TC260），直接输出 🤖 99%。

---

## 去水印流程（`--remove-watermark`）

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
     ├─ 边缘清理（blendEdge / progressive inpaint）
     └─ 4 轮迭代或 residual ≤ 0.25 停止
  
  4. 重编码保存（JPEG Q95 / PNG）
```

---

## 加水印流程（`--add-watermark`）

```
  1. 确定 producer
     ├─ 已知名称（gemini/doubao/jimeng）
     └─ 自定义文字 → 用 basicfont.Face7x13 渲染到 RGBA
  
  2. 计算位置
     ├─ 已知：PositionResolver（或 Gemini catalog）
     └─ 自定义：右下角 10px margin + 按宽高比缩放
  
  3. 叠加
     result = alpha * logo + (1 - alpha) * original
  
  4. 注入元数据（仅 doubao/jimeng）
     └─ PNG text chunk "TC260": {"Label":"1","ContentProducer":"doubao"}
  
  5. 保存为 PNG
```

---

## 位置确定策略

### Gemini — 目录优先 + 动态搜索

1. **精确匹配**：如果图片尺寸在 `officialGeminiSizes` 目录中，直接使用目录中的 watermark 大小和边距
2. **近官方投影**：如果宽高比最接近的目录项偏差 < 15%，投影缩放 watermark 大小和边距
3. **通用回退**：按图片短边缩放 96px / 48px / 36px（相对 2048px 基准）
4. **NCC 搜索**：以上述种子位为中心，±50px（或 5% 宽）位置搜索，±30% 尺寸搜索

### 文字水印 — 几何定位 + NCC 对齐

1. **PositionResolver**：基于图片 `min(w,h)` 按比例缩放 alpha map 尺寸（2048px 基准）
2. **二值掩码提取**：在预期位置周围 ±60px 区域提取 bright + low-sat + tophat 像素
3. **NCC 对齐搜索**：11 个 scale step（[0.6, 1.4]）× coarse stride 4 + fine stride 1
4. **回退**：白色/浅色背景上 NCC 对齐失败（tophat < 12）→ 直接用 PositionResolver 位置

---

## 添加新水印模型的步骤

### 1. 准备 Alpha Map

```bash
# 从参考项目或自制捕获图中提取 alpha PNG
python scripts/generate_alpha_go.py model_alpha.png modelAlphaRaw \
    --pkg watermark \
    --output internal/watermark/model_alpha.go \
    --comment "MyModel watermark, 200x50, captured at 1024px"
```

`scripts/` 目录下已有参考 alpha 资产：
- `doubao_alpha.png` (335×83)
- `jimeng_alpha.png` (414×118)
- `gemini_bg_96.png` (96×96)
- `gemini_bg_48.png` (48×48)

### 2. 注册配置

创建 `internal/watermark/model.go`：

```go
func init() {
    // 加载 alpha map 数据
    data := make([]float64, 200*50)
    for i := 0; i < 200*50; i++ {
        data[i] = modelAlphaRaw[i]
    }
    am := NewAlphaMap(200, 50, data)

    Register(Config{
        Type:            TypeMyModel,
        Name:            "mymodel",
        AlphaMap:        am,
        LogoColor:       [3]float64{255, 255, 255},
        DetectThreshold: 0.35,
        PositionResolver: func(w, h int) []Position {
            // 根据图片宽高计算水印位置
            shorter := w
            if h < shorter { shorter = h }
            scale := float64(shorter) / 2048
            // ...
        },
    })
}
```

### 3. 添加类型常量

在 `internal/watermark/types.go` 中：

```go
const (
    TypeUnknown Type = iota
    TypeGeminiSparkle
    TypeDoubao
    TypeJimeng
    TypeMyModel  // ← 新增
)
```

### 4. 添加参数配置

在 `internal/watermark/detect_helpers.go` 中：

```go
func DefaultMyModelParams() TextMarkParams {
    return TextMarkParams{
        MaxSaturation:  55,
        LogoMinLuma:    150,
        TophatDelta:    12,
        MorphOpenSize:  5,
        AlignSearchMin: 0.60,
        AlignSearchMax: 1.40,
    }
}
```

在 `paramsForConfig` 中添加分支。

### 5. 更新 CLI

在 `cmd/detect.go` 中：
- 在 `init()` 的 `--producer` 帮助文本中添加新名称
- 在 `service/detect.go` 中添加 `ProviderMyModel` 常量
- 在 `ProduceToConfig` 中添加子串匹配

### 6. 更新脚本 alpha 资产

将模型的 alpha PNG 复制到 `scripts/` 目录供后续参考。

---

## 参考项目

本实现参考了以下开源项目：

- **[gemini-watermark-remover](https://github.com/GargantuaX/gemini-watermark-remover)** — Gemini Sparkle 水印的 alpha map 数据和尺寸目录
- **[remove-ai-watermarks](https://github.com/wiltodelta/remove-ai-watermarks)** — 豆包/即梦文字水印的 alpha map 资产、分数定位参数和 `_text_mark_engine.py` 共享引擎
- **[doubao-watermark-remover](https://github.com/zhengsuanfa/doubao-watermark-remover)** — 豆包水印的简单背景替换方案

---

## 局限性

- 逆 alpha 混合在纯色/渐变背景上效果最好，复杂纹理区域可能有轻微残影
- 文字水印的 alpha map 在 2048px 基准宽度下训练，大幅偏离此尺寸的图片可能需扩大搜索范围
- 仅支持叠加型可见水印（白/灰半透明覆盖），不支持植入型/隐写水印
- Gemini C2PA 元数据需要数字签名，加水印时无法伪造（仅注 TC260）
