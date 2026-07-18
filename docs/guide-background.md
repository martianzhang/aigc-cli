# 背景处理

`aigc-cli background` 提取图片中的主体，去除或替换背景。专为 **AI 生成的纯色背景图**（常为白底、渐变底或纯色底）优化，也可用于普通抠图场景。

> **核心算法**：基于 CIELAB ΔE 色度键控（Chroma Key），全纯 Go 实现，无需外部模型或 API。

---

## 快速开始

```bash
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

# AI 辅助去背（先用大模型压平背景，再精确去背）
aigc-cli background input.png --remove --ai default
aigc-cli background input.png --remove --ai "Keep subject on pure white background"

# 批量处理
aigc-cli background *.png --remove
```

输出文件：

```
input.png  --remove                    → input_removebg.png    # 透明背景 PNG
input.png  --replace "#FF0000"         → input_replaced.png    # 红底 PNG
input.png  --replace ./bg.jpg          → input_replaced.png    # 合成后 PNG
input.png  --remove --autocrop         → input_removebg.png    # 紧贴主体的透明 PNG
```

---

> 💡 **提示：给 AI 的生图提示词加一句 "纯色背景"**
>
> 本工具对**纯色背景**（白、黑、灰、纯蓝等）效果最好。生成图片时在 prompt 里加一句
> `"solid color background"`、`"pure white background"` 或 `"plain background"`，
> 避免使用 `"gradient"`、`"bokeh"`、`"natural lighting"` 这类词，
> 能让去背效果大幅提升。AI 常见的渐变背景（灯光渐变、天空渐变等）
> 需要多色建模来覆盖，是本工具当前的技术瓶颈。

---

## 算法原理

### 三步提取流程

```
输入图片
  │
  ▼
① 背景色自动检测
   ├── 采样边缘区域（默认图像外沿 5% 宽度）
   ├── K-Means 聚类（k=2~3）分离背景/前景
   └── 取最大簇中心作为主背景色
  │
  ▼
② Alpha 遮罩生成
   ├── 逐像素计算 CIELAB ΔE 到背景色距离
   ├── 三阈值映射：前景（确定保留）| 过渡（半透明）| 背景（透明）
   └── 边缘羽化（Feathering）消除锯齿
  │
  ▼
③ 输出
   ├── --remove：PNG RGBA（alpha 通道）
   ├── --replace：合成到新背景
   └── --autocrop：紧贴主体边界裁剪（可选，与 --remove/--replace 配合）
```

### CIELAB ΔE 色差

核心是 CIELAB 色彩空间的色差公式（CIE76），比 RGB 欧氏距离更符合人眼感知：

```
ΔE = √((L₁-L₂)² + (a₁-a₂)² + (b₁-b₂)²)
```

| ΔE 值 | 人眼感受 | 遮罩处理 |
|---|---|---|
| 0-2 | 基本看不出差异 | 背景（完全透明） |
| 2-10 | 细微差异，训练过的眼睛才能分辨 | 背景（完全透明） |
| 10-30 | 普通人不难看出差异 | 半透明过渡（边缘羽化区） |
| 30+ | 明显不同的颜色 | 前景（完全不透明） |

---

## 参数一览

### 核心操作参数

| 参数 | 说明 | 默认 |
|---|---|---|---|
| `<file...>` | 输入图片路径（PNG/JPEG/WebP/BMP，支持批量） | 必填 |
| `--remove` | 去除背景，输出透明 PNG | `false` |
| `--replace` | 替换背景：HTML 颜色（`#RRGGBB`）或图片路径 | `""` |
| `--ai` | AI 辅助去背：先用大模型把背景压成纯色，再精确去背。内置模板：`default` / `white` / `human` / `product` / `good`，也支持自定义提示词 | `""` |

> `--remove` 和 `--replace` 互斥，同时指定时 `--replace` 优先。
> `--ai` 需要配合 `--remove` 使用，调用 `aigc-cli image` 进行生成，需配置 API Key 和 model。

### 裁剪参数（让输出紧贴主体）

| 参数 | 短参 | 说明 | 默认值 |
|---|---|---|---|
| `--autocrop` | `--ac` | 裁剪到主体边界框，不留多余透明区域 | `false` |
| `--padding` | | 主体四周留白（px）。单值 `20` 表示四边统一；四值 `"10,20,10,20"` 表示上右下左 | `0` |
| `--aspect-ratio` | `--ar` | 强制输出宽高比，如 `"1:1"`、`"16:9"`、`"3:4"`。只扩展画布不缩放主体 | `""`（不限制） |

### 调优参数（通常不需要动）

| 参数 | 说明 | 默认值 | 何时需要调整 |
|---|---|---|---|
| `--tolerance` | 色彩容差。与背影色的最大 ΔE 距离，超过此值视为前景 | `auto`（动态计算） | 背景渐变严重时调大；背景与主体颜色接近时调小 |
| `--feather-radius` | 边缘羽化半径（px） | `auto`（≈ 图像对角线的 0.3%） | 边缘锯齿明显时调大；需要锋利边缘时设为 0 |
| `--bg-color` | 手动指定背景色，格式 `#RRGGBB` | `auto`（自动检测） | 自动检测不准时手动指定（如多色背景或背景占比很小） |
| `--fg-threshold` | 前景保护阈值（ΔE 乘数）。ΔE > tolerance × threshold 的像素强制不透明 | `1.5` | 主体边缘被意外抠掉时调低；背景残留时调高 |
| `--smooth` | Alpha 遮罩平滑迭代次数 | `1` | 遮罩噪点多时设为 2-3；需保持细节时设为 0 |
| `--erode-radius` | 腐蚀半径（px），从边缘向内收缩，去除背景残留 | `0` | 主体边缘有背景色光晕时调大 |
| `--close-radius` | 形态学闭合半径（px），先膨胀再腐蚀，填掉主体内部颜色相近的"孔洞" | `0` | 主体与背景颜色接近时设 4-6 |

### 投影参数

| 参数 | 说明 | 默认值 |
|---|---|---|
| `--shadow` | 启用投影 | `false` |
| `--shadow-offset` | 投影偏移 `"dx,dy"`（px） | `"4,4"` |
| `--shadow-blur` | 模糊半径（px） | `6` |
| `--shadow-color` | 投影颜色（hex） | `"#000000"` |
| `--shadow-opacity` | 不透明度 0-100 | `40` |

### 调试与输出参数

| 参数 | 说明 | 默认值 |
|---|---|---|
| `--mask-only` | 输出黑白遮罩图，用来查看 AI 识别出的主体范围，方便调试参数 | `false` |
| `--json` | JSON 格式输出（详情、耗时等） | `false` |
| `--preview` | 完成后自动打开系统预览 | `false` |
| `--output` | 输出目录（默认为输入文件所在目录） | `""` |

---

## 自动调优详解

所有 `auto` 默认值不是固定常量，而是根据每张图片动态计算：

### tolerance 自动计算

```
1. 取边缘采样区的像素集合 S
2. 计算 S 与背景色之间的 ΔE 均值和标准差
3. tolerance = mean(ΔE_S) + 2.5 × std(ΔE_S)
```

这样遇到轻微渐变的背景（AI 图常见），容差会自动扩大；遇到干净的纯色背景，容差会收窄。

**极端情况自适应**：

| 背景特征 | mean(ΔE_S) 表现 | 自动 tolerance 行为 |
|---|---|---|
| 完美纯色 | ≈ 0-2 | 3-5，非常严格 |
| 轻微渐变 | 5-15 | 适度放宽 |
| 强渐变/光晕 | 15-40 | 大幅放宽，并输出 WARN 提示 |

### feather 自动计算

```
feather = clamp(√(width² + height²) × 0.005, 1px, 8px)
```

小图（~512px）≈ 2px，大图（~4096px）≈ 8px。用户设置 `--feather-radius 0` 可禁用羽化。

---

## 自动裁剪（--autocrop）

抠完图主体只占画面一小块，透明区域太多，是日常最烦的问题。`--autocrop` 自动紧贴主体裁剪。

### 效果对比

```
原图 1024×1024                --autocrop 后
┌──────────────────────┐     ┌────────────┐
│                      │     │            │
│                      │     │    👩      │
│        👩            │  →  │            │
│                      │     │            │
│                      │     └────────────┘
└──────────────────────┘     480×720
透明区域占了 70%              紧贴人物
PPT 里还要手动裁剪
```

### 边界框计算

```
① 扫描 alpha 遮罩，找到非透明像素的最小/最大 x、y
② 得到主体边界框 [x_min, y_min, x_max, y_max]
③ 加上 --padding 向外扩展
④ 如果指定了 --ratio，按比例扩展画布（不拉伸主体）
```

### --padding 详解

控制主体到画布边缘的间距，单位像素：

```bash
# 默认无 padding（紧贴主体边缘）
aigc-cli background input.png --remove --autocrop

# 上下左右统一 30px
aigc-cli background input.png --remove --autocrop --padding 30

# 上 20 右 40 下 20 左 40（不对称留白）
aigc-cli background input.png --remove --autocrop --padding "20,40,20,40"
```

> `padding` 的顺序是：上 右 下 左（顺时针），和 CSS 的 `padding` 顺序一致。

### --aspect-ratio / --ar 详解

强制输出画布比例，**主体不会变形**，画布向四周扩展来填满比例：

```
主体边界框 400×600（宽高比 2:3）

--aspect-ratio "1:1"（或 --ar "1:1"）→ 画布扩展到 600×600，主体居中
  ┌──────────────────────┐
  │                      │
  │       👩 主体         │
  │       居中            │
  │                      │
  └──────────────────────┘
  600×600

--aspect-ratio "16:9"（或 --ar "16:9"）→ 画布扩展到 1067×600，主体居中
  ┌────────────────────────────────────┐
  │                                    │
  │              👩 主体                │
  │              居中                   │
  │                                    │
  └────────────────────────────────────┘
  1067×600
```

```bash
# 方形头像（小红书/Ins 风格）
aigc-cli background portrait.png --remove --ac --ar "1:1" --padding 20

# PPT 网格排版，统一 3:4
aigc-cli background product.png --remove --ac --ar "3:4" --padding 30

# 横幅封面
aigc-cli background photo.png --replace "#FF0000" --ac --ar "16:9"
```

### --padding 与 --aspect-ratio 的配合

```
先算主体边界框
    ↓
+ padding 扩展
    ↓
┌──────────────────┐
│  │  ─主体─  │    │
│  │         │    │
└──────────────────┘
    ↓
如果指定了 --aspect-ratio（或 --ar），继续向外扩展画布直到符合比例
    ↓
┌────────────────────────┐
│    │  ─主体─  │        │
│    │         │        │
└────────────────────────┘
结果：主体始终居中，padding 是到主体边界框的最小间距，
      aspect-ratio 只额外扩展不会缩小。
```

---

## 实用场景

### AI 白底图提取主体做 PPT

```bash
# AI 生成了产品白底图 → 去背景放 PPT
aigc-cli background product.png --remove

# 如果自动效果不完美，稍调容差
aigc-cli background product.png --remove --tolerance 15

# 再不满意，手动指定背景色（纯白）
aigc-cli background product.png --remove --bg-color "#FFFFFF"
```

### 替换背景为品牌色

```bash
# 品牌主色 #1A73E8
aigc-cli background photo.png --replace "#1A73E8"
```

### 合成到风景背景

```bash
# 主体 from subject.png，背景 from bg.jpg
aigc-cli background subject.png --replace ./bg.jpg
```

### 批量换背景

```bash
# 一批产品图统一换白底
for f in product_*.png; do
  aigc-cli background "$f" --replace "#FFFFFF"
done
```

### 提取人物做 PPT 头像

```bash
# 人物照去背景 + 裁剪到紧贴 + 正方形 1:1 + 留 20px 边
aigc-cli background portrait.png --remove --ac --ar "1:1" --padding 20

# 输出 portrait_removebg.png，直接拖进 PPT，不用二次裁剪
```

### 统一产品图尺寸做页面网格

从 AI 生了一批 1:1 白底产品图，但产品形状不同（高的、矮的、方的），想统一排版：

```bash
# 不加 --ac：每张图裁剪到各自的产品大小，尺寸不统一
aigc-cli background product_*.png --remove --ac

# 加 --ar "3:4"：所有产品强制统一画布比例，胶卷装帧效果
aigc-cli background product_*.png --remove --ac --ar "3:4" --padding 30
```

### 两步法：AI 压平背景 → 精确去背

对于复杂背景（光照渐变、纹理背景等），`background` 的纯色键控可能不够干净。
可以先让 AI"重绘"一张纯色背景图，再精确去背：

```bash
# 第一步：用 image 命令重绘，把背景压成纯色
# --image-url 传入原图，--prompt 要求保持主体、背景变纯色
# --output-format png 确保输出无损 PNG
# --model 可以换你自己常用的模型
aigc-cli image \
  --model "openai/dall-e-3" \
  --image-url complex_bg.png \
  --prompt "Regenerate exactly the same subject on a pure white solid background, no shadows, no gradient, flat lighting" \
  --output-format png \
  -o /tmp/

# 第二步：对重绘结果做精确去背
aigc-cli background /tmp/complex_bg_0.png --remove --ac --padding 20
```

**提示词模板（中英文都支持）**：

| 场景 | 提示词 |
|---|---|
| 通用 | `"Regenerate exactly the same subject on a pure white solid background, no shadows, no gradient, flat uniform lighting"` |
| 产品 | `"Same product on a solid pure white background, studio lighting, no shadows"` |
| 人像 | `"Same person on a solid pure white background, keep all details of the clothing and hair"` |
| 中文 | `"保持主体完全不变，把背景替换为纯白色，不要阴影和渐变"` |

```bash
# 输出遮罩图，用系统看图软件打开，看看 AI 的"识别结果"
aigc-cli background input.png --mask-only --preview
```

你会看到一张黑白图：

| 遮罩颜色 | 意思 | `--remove` 会对它做什么 |
|---|---|---|
| 🟩 **白色** | AI 确认是主体 | 保留（不透明） |
| ⬛ **黑色** | AI 确认是背景 | 去掉（透明） |
| 🟨 **灰色** | 边缘过渡区，AI 拿不准 | 半透明（羽化过渡） |

**什么时候用这个**：自动抠图效果不满意时，先跑 `--mask-only` 看看 AI "怎么想的"：

- 猫身上有黑块 → `--fg-threshold` 调低，减少主体被误抠
- 背景残留了白斑 → `--tolerance` 调低，让背景识别更严格  
- 边缘太生硬 → `--feather-radius` 调大
- 边缘有光晕 → `--erode-radius` 调大

---

## 进阶技巧

### 渐变背景处理

AI 图常见边缘渐变背景（如暗角、径向渐变）。自动 tolerance 会扩大，但仍建议：

1. 使用 `--tolerance` 稍调大
2. 用 `--erode-radius 2-3` 腐蚀掉残余的光晕

```bash
aigc-cli background gradient_bg.png --remove --erode-radius 2
```

### 主体颜色与背景相近

如果主体的颜色和背景很接近（如白猫在雪地），ΔE 区分度低。这时：

1. `--bg-color` 手动指定背景色（如果已知）
2. `--fg-threshold 1.2` 降低前景保护阈值，减少主体侵蚀
3. 考虑用 `--mask-only` 输出遮罩，人眼确认后再手动调整

### 多色背景

少数 AI 图有多色渐变或图案背景，自动 K-Means 会选主要颜色。如果效果不佳：

1. 先手动指定背景色：`--bg-color`
2. 或增大 `--sample-region 10` 扩大采样区
3. 极端情况建议用外部专业工具（Photoshop / remove.bg）

---

## 技术局限性

| 限制 | 说明 |
|---|---|---|
| **透明物体** | 玻璃杯、纱巾等半透明物体的透明度不会被保留 |
| **背景与主体同色** | 色差太小无法区分，需手动 `--bg-color` |
| **复杂背景** | 非纯色背景（照片实景）用本工具效果差，非本工具适用场景 |
| **发丝级细节** | 单像素宽的发丝可能丢失，建议配合 `--feather-radius 0` 精修 |
| **autocrop 与孤立噪点** | 遮罩中残留的孤立噪点会把边界框撑大，建议配合 `--erode-radius` 先清理 |

---

## 与 detect 命令的关系

`background` 和 `detect` 各司其职：

| 命令 | 职责 | 适用场景 |
|---|---|---|
| `detect` | 分析图片：AIGC 检测、水印检测去除、元数据查看 | 分析是不是 AI 图、去除水印 |
| `background` | 操作图片：去背景、换背景 | 抠图、合成、P PT 素材 |

**配合使用**：先用 `detect` 确认图片的元数据和格式，再用 `background` 处理：

```bash
# 先看看图片信息
aigc-cli detect input.png

# 再去背景
aigc-cli background input.png --remove
```

---

## 常见问题

### 为什么输出是 PNG？不能是 JPEG 吗？

透明背景需要 alpha 通道，只有 PNG 和 WebP 支持。JPEG 不支持透明，所以 `--remove` 和 `--mask-only` 只能输出 PNG。

`--replace` 输出也是 PNG（包含合成后的完整图像）。

### 支持 WebP 吗？

输入支持 WebP（通过 `golang.org/x/image/webp`），输出统一为 PNG。

### 可以去掉非纯色背景吗？

算法基于纯色假设。如果背景很复杂（照片实景、纹理背景），请用专业工具（Photoshop 的"主体选择"、remove.bg API 等）。

### 速度怎么样？

纯 Go、纯 CPU、无 GPU 依赖。一张 1024×1024 图从头到尾 < 100ms。

### --ratio 和 --padding 的区别是什么？

`--padding` 控制主体到画布边缘的**最小间距**，`--ratio` 控制画布的**宽高比**。

执行顺序是：先算出主体边界框 → 加上 padding → 再按 ratio 扩展画布（只扩展不缩小）。

### 为什么用 --ratio 后主体没有居中？

永远居中的。`--ratio` 只**向外扩展画布**，不缩放主体。如果最终看到主体偏了，检查一下边界框之外是否有残留的孤立的非透明像素（如噪点），它们会把边界框撑大。

### 什么情况不加 autocrop？

- 需要保持原始坐标的场景（如图层叠加、蒙版对齐）
- 图片中主体本身就占满画布（加了也没区别）

### 怎么知道背景检测到什么颜色？

```bash
aigc-cli background input.png --remove --json
```

JSON 输出中包含 `detected_bg_color` 字段，告知自动检测到的背景色的 RGB 值。
