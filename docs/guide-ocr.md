# OCR 文字识别

> 📝 离线文字识别（Optical Character Recognition），基于 ONNX Runtime 本地推理，无需联网、无需 API Key，数据不出设备。

---

## 技术选型（已确定）

| 决策项 | 结论 | 理由 |
|---|---|---|
| **OCR 引擎** | RapidOCR (PP-OCRv5) | ONNX 格式、社区评价好、CJK 支持最强 |
| **管线架构** | det → rec 两阶段流水线 | RapidOCR 成熟方案，可独立调试各阶段 |
| **语言支持** | 中英同步 | RapidOCR 中英文模型均有 ONNX 格式，一起做 |
| **表格/版面** | ✅ 需要。RapidOCR 不内置，用 PP-StructureV3 独立 ONNX 模型补充 | PaddlePaddle 有 PP-DocLayout、SLANet 等 ONNX 格式 |
| **PDF 输入** | ✅ 内嵌，用 `go-fitz`（MuPDF 封装）PDF→图片 | 项目已有 CGO 模式（audio/sherpa-onnx），`go-fitz` 最成熟 |
| **ONNX Runtime** | `pure-onnx`（纯 Go，无 CGO） | 复用现有 `internal/rmbg` 的模式 |
| **CGO 范围** | 仅 PDF 渲染需要 CGO，OCR 推理本身零 CGO | OCR 核心保持纯 Go |

---

## 可用 OCR 方案概览

2025-2026 年主流离线 OCR 引擎对比如下：

| 方案 | 架构 | 语言支持 | GPU 要求 | 布局能力 | 体积 | 许可证 | 特点 |
|---|---|---|---|---|---|---|---|
| **RapidOCR** ⭐ | PP-OCRv4/v5 (ONNX) | 80+ (强 CJK) | 可选 | OCR 仅识别（无布局分析） | ~20 MB | Apache 2.0 | **选型确认**。ONNX 格式，多语言部署，最快 |
| **PaddleOCR** | PP-OCRv5 (PaddlePaddle) | 80+ (强 CJK) | 推荐 | 优秀 — PP-StructureV3 表格/布局 | ~150 MB | Apache 2.0 | 训练引擎+全功能套件，但不直接 Go 可用 |
| **Surya OCR** | VLM 650M | 90+ | 推荐 (CPU 可用) | 优秀 — 端到端布局+识别 | ~200 MB | OpenRAIL | 远期增强候选，ONNX 导出尚不成熟 |
| **Tesseract 5.x** | LSTM | 100+ | 否 (CPU only) | 弱 | ~10 MB | Apache 2.0 | 备选，CPU 原生，清晰印刷体 |
| **EasyOCR** | CRAFT + CRNN | 80+ | 可选 | 中等 | ~500 MB | Apache 2.0 | 备选，手写体较好 |

> 结论：**RapidOCR 做主力**，PP-StructureV3 补充表格/版面分析，远期可考虑 Surya 增强。

### RapidOCR 与 PP-StructureV3 的关系

RapidOCR **只包含 OCR 本身**（detection + recognition），不内置布局分析。版面分析的 ONNX 模型独立提供：

| 能力 | RapidOCR（P0） | PP-DocLayout（P2） |
|---|---|---|
| 文本检测 | ✅ `ch_PP-OCRv5_det.onnx` | — |
| 文字识别 | ✅ `ch_PP-OCRv5_rec.onnx` | — |
| 方向分类 | ✅ `ch_ppocr_mobile_v2.0_cls.onnx` | — |
| 版面分析（text/table/figure） | ❌ 不内置 | ✅ `PP-DocLayout-L.onnx`（PaddlePaddle HF） |
| 非文本区截图嵌入（table/figure） | ❌ 不内置 | ✅ PP-DocLayout 给坐标 → 直接截图 |

---

## 设计目标

- **纯离线**：所有推理本地完成，ONNX Runtime 加载模型，不依赖任何外部 API
- **Go 原生**：OCR 核心复用 `pure-onnx`（无 CGO），PDF 渲染通过 `go-fitz`（CGO，与 audio 模式一致）
- **模型即插即用**：通过 `ocr init` 下载模型，支持多模型切换
- **中英同步支持**：默认同时下载中英文模型，`--lang` 参数切换
- **PDF 全流程**：PDF → 图片 → OCR，一步到位
- **表格/版面**（P2）：识别非文本区域（表格、图片）自动截图嵌入 Markdown

---

## 命令行设计

```bash
# 初始化/下载 OCR 模型（默认中英文全部下载）
aigc-cli ocr init
aigc-cli ocr init --model rapidocr-zh       # 仅中文
aigc-cli ocr init --model rapidocr-en       # 仅英文

# 列出可用/已安装模型
aigc-cli ocr init --list
aigc-cli ocr init --list-installed

# 图片/PDF → Markdown（默认）
aigc-cli ocr scan image.png                  # 输出 image.md
aigc-cli ocr scan --lang en doc.png          # 指定英文
aigc-cli ocr scan -o result.md photo.jpg     # 指定输出路径

# PDF 识别（自动逐页转图→OCR）
aigc-cli ocr scan document.pdf               # 输出 document.md
aigc-cli ocr scan --pages 1-3 doc.pdf       # 指定页码范围

# 从 stdin 读取
cat document.png | aigc-cli ocr scan

# JSON 输出（含位置坐标和置信度，用于调试/自定义处理）
aigc-cli ocr scan --json receipt.jpg


```

### 输出命名

```
默认命名: input_name.md          （如果 input_name 是 image.png）
冲突时:   ocr_1712345678.md      （当前时间戳）

通过 -o 指定: 直接使用给定路径
```

### 输出示例（Markdown）

```
# 文档标题

这是第一段文字内容。OCR 识别出的文本段落以 Markdown 格式输出。

## 表格区域

对于表格、图片等非纯文本区域，自动截图并嵌入 Markdown：

![table-1](image_assets/image_page1_table1.png)
*表格 1：销售数据*

## 图表

![figure-1](image_assets/image_page1_figure1.png)
*图 1：季度增长趋势*

> 注：表格和图片区域的文字不会被 OCR 识别，而是保留为截图，
> 确保信息的原始样貌和布局完全保真。
```

如需纯 JSON 格式用于二次处理，通过 `--json` 输出行级坐标和置信度：

```json
{
  "text": "上海市浦东新区张江高科技园区\n招商银行股份有限公司上海分行\n(人民币) 壹佰万元整",
  "pages": [
    {
      "page": 0,
      "lines": [
        {
          "text": "上海市浦东新区张江高科技园区",
          "bbox": [345, 120, 890, 160],
          "confidence": 0.982
        }
      ]
    }
  ]
}
```

---

## 模型方案

### RapidOCR ONNX 模型

| 组件 | 模型文件 | 大小 | 来源 |
|---|---|---|---|
| **文本检测**（det） | `ch_PP-OCRv4_det_infer.onnx` | ~5 MB | SWHL / RapidOCR HF |
| **文字识别**（rec） | `ch_PP-OCRv4_rec_infer.onnx` | ~11 MB | SWHL / RapidOCR HF |
| **方向分类**（cls） | `ch_ppocr_mobile_v2.0_cls_infer.onnx` | ~0.6 MB | SWHL / RapidOCR HF |
| **英文字典** | `dict_en.txt` | ~20 KB | monkt/paddleocr-onnx HF |
| **中文字典** | `dict_zh.txt` | ~200 KB | monkt/paddleocr-onnx HF |

> 英文检测模型语言无关（det），复用中文 det 即可。识别模型需要按语言切换。

### PP-DocLayout 模型（P2 扩展）

| 组件 | 模型文件 | 大小 | 类别数 | 来源 |
|---|---|---|---|---|
| **版面检测** | `PP-DocLayout-L.onnx` | ~50 MB | 10 类 | PaddlePaddle HF |
| **版面检测（精细）** | `PP-DocLayoutV2.onnx` | ~213 MB | 25 类 | ppu-paddle-ocr-models |

推荐先用轻量版 PP-DocLayout-L（10 类：paragraph_title / text / figure / figure_caption / table / table_title / header / footer / reference / equation），够用。

---

## 技术方案

### 架构概览

```
┌─────────────────────────────────────────────────────┐
│                     cmd/ocr.go                       │
│         命令定义、flag 注册、RunOCR/Init               │
└──────────┬──────────────────────────┬──────────────┘
           │                          │
    ┌──────▼──────┐          ┌───────▼────────┐
    │ internal/   │          │ internal/pdf/   │
    │ ocr/        │          │ (go-fitz)       │
    │             │          │ PDF→image       │
    │ OCR 推理    │          │ CGO (MuPDF)     │
    │ pure-onnx   │          └────────────────┘
    │ 无 CGO      │
    └──────┬──────┘
           │
    ┌──────▼──────────────────────────────┐
    │      ONNX Runtime (pure-onnx)        │
    │  det.onnx → rec.onnx → cls.onnx     │
    └─────────────────────────────────────┘
```

### 推理流程

```
输入图片 / PDF（go-fitz 渲染）
    ↓
┌─ 版面分析（P2, PP-DocLayout）
│  → 输出区块：text / table / figure / title
│     ↓ text 区块进入 OCR 管线
│     ↓ table / figure 区块 → 截图嵌入 Markdown
│
├─ 方向分类（cls.onnx, 可选）
│  → 纠正 90°/180° 旋转
│
├─ 文本检测（det.onnx -> DBNet）
│  → 输出概率图 + 文字框（四点坐标）
│
├─ 后处理（DB 二值化 → 连通域 → NMS → 文字框）
│
├─ Perspective Crop（仿射变换裁切文字区域）
│
├─ 文字识别（rec.onnx -> CRNN/SVTR）
│  → 输出 CTC 解码后文本
│
└─ 结构化输出（行号 + 文本 + confidence + bbox）
```

### 文件结构

```
internal/
├── ocr/                        # OCR 核心（det + rec，pure-onnx，无 CGO）
│   ├── ocr.go                  # OCREngine 核心类型 + 接口
│   ├── detect.go               # 文本检测（DBNet ONNX 推理）
│   ├── detect_preproc.go       # 检测预处理（resize、normalize）
│   ├── detect_postproc.go      # 检测后处理（DB 二值化、连通域、NMS）
│   ├── rec.go                  # 文字识别（CRNN/SVTR ONNX 推理）
│   ├── rec_preproc.go          # 识别预处理（resize、padding、normalize）
│   ├── rec_postproc.go         # 识别后处理（CTC 解码、字典映射）
│   ├── cls.go                  # 方向分类（可选）
│   ├── pipeline.go             # 管线编排（det → rec）
│   ├── models.go               # 模型注册表 + 下载管理
│   └── ocr_test.go             # 测试
│
├── layout/                     # 版面分析（P2, pure-onnx）
│   ├── detector.go             # PP-DocLayout ONNX 推理
│   └── types.go                # LayoutBlock 类型定义
│
└── pdf/                        # PDF→图片（CGO, go-fitz）
    └── render.go               # PDF 逐页渲染为 image.Image
```

### Go 参考项目

| 项目 | ⭐ | 可参考的部分 | 差异点 |
|---|---|---|---|
| **multippt/gopaddleocr** | 2 | 架构设计（engine/workflow/detect/recognize） | 用 `yalue/onnxruntime_go`（CGO），我们改用 `pure-onnx` |
| **weihuanwan/paddleocr-go** | 9 | PP-Structure 版面/表格实现（P2 参考） | 用 GOCV（OpenCV CGO），算法可抄 |
| **LKKlein/paddleocr-go** | 83 | 预处理/后处理参考 | 太老（2020），PaddlePaddle 1.8 |

> 不能直接 import 这些包（ONNX Runtime 接口不同），但预处理/后处理算法可以直接参考移植。

### 核心接口

```go
package ocr

// OCRLine 单行识别结果
type OCRLine struct {
    Text       string    `json:"text"`       // 识别文字
    BBox       [4][2]int `json:"bbox"`       // 四点坐标 [左上, 右上, 右下, 左下]
    Confidence float32   `json:"confidence"` // 置信度 0-1
}

// OCRPage 单页结果
type OCRPage struct {
    Page  int       `json:"page"`  // 页码（0-based）
    Lines []OCRLine `json:"lines"` // 该页所有行
}

// OCRResult 完整识别结果
type OCRResult struct {
    Pages []OCRPage `json:"pages"` // 所有页
    Text  string    `json:"text"`  // 全文拼接
}

// Engine 管理 ONNX Runtime 会话和推理管线
type Engine struct {
    det     *onnx.Session  // 检测模型
    rec     *onnx.Session  // 识别模型
    cls     *onnx.Session  // 方向分类器（可选）
    libPath string         // ONNX Runtime 库路径
}

func NewEngine(libPath, detModel, recModel string) (*Engine, error)
func (e *Engine) Scan(img image.Image) (*OCRResult, error)
func (e *Engine) ScanFile(path string) (*OCRResult, error)  // 自动识别 PDF/图片
func (e *Engine) Close()
```

### 模型下载管理

遵循现有 `audio init` / `background init` 模式：

```go
// models.go

type OCRModelPack struct {
    ID      string   `json:"id"`      // "rapidocr-zh", "rapidocr-en"
    Name    string   `json:"name"`    // "RapidOCR 中文"
    Lang    string   `json:"lang"`    // "zh", "en"
    Models  []ModelFile               // det.onnx, rec.onnx, cls.onnx
    DictURL string   `json:"dict_url"` // 字典文件 URL
}

type ModelFile struct {
    Type   string `json:"type"`   // "det", "rec", "cls"
    URL    string `json:"url"`    // 下载 URL（.onnx）
    Size   int64  `json:"size"`   // 文件大小
    SHA256 string `json:"sha256"` // 校验和
}
```

模型下载路径：`~/.config/aigc-cli/models/ocr/{lang}/`

---

## PDF 支持（go-fitz）

### 选型理由

| 方案 | 方式 | CGO | 成熟度 |
|---|---|---|---|
| **go-fitz**（MuPDF 封装） | ✅ 直接 API 调用 | 是 | ⭐⭐⭐⭐⭐ 最成熟，1.5k ⭐ |
| `pdftoppm`（poppler） | subprocess | 否 | 依赖系统安装 |
| `pdfcpu` | 纯 Go | 否 | 无渲染能力 |
| `unipdf` | 商业 | 否 | 贵 |

go-fitz 是 Go 生态最成熟的 PDF→image 方案，API 简洁：

```go
import "github.com/gen2brain/go-fitz"

doc, _ := fitz.New("document.pdf")
defer doc.Close()

for n := 0; n < doc.NumPage(); n++ {
    img, err := doc.Image(n)  // *image.RGBA
    // → 传给 OCR Engine.Scan()
}
```

项目已有 CGO 模式（audio/sherpa-onnx），go-fitz 的 CGO 依赖在同一个可接受范围内。

---

## 实现计划

### P0 - OCR 核心（~5 天）

| 任务 | 文件 | 依赖 |
|---|---|---|
| 模型注册表 + 下载管理 | `internal/ocr/models.go` | — |
| ONNX Session 封装 | `internal/ocr/ocr.go` | `pure-onnx` |
| 检测预处理（resize, normalize） | `internal/ocr/detect_preproc.go` | — |
| DBNet ONNX 推理 | `internal/ocr/detect.go` | 检测模型 |
| 检测后处理（DB 二值化, NMS, 连通域） | `internal/ocr/detect_postproc.go` | 检测输出 |
| 识别预处理（crop, resize, padding） | `internal/ocr/rec_preproc.go` | 检测输出 |
| CRNN/SVTR ONNX 推理 + CTC 解码 | `internal/ocr/rec.go` | 识别模型 |
| 管线编排 | `internal/ocr/pipeline.go` | det + rec |
| `cmd/ocr.go` 命令定义 + init + scan | `cmd/ocr.go` | 管线 |

### P1 - 中英同步 + PDF + Markdown 输出 + MCP（~4 天）

| 任务 | 内容 |
|---|---|
| 英文模型注册 + 字典 | 加入 `en_PP-OCRv4_rec_infer.onnx` + 英文字典 |
| `--lang` 参数切换 | 选择对应语言模型 |
| PDF 渲染 | `internal/pdf/render.go` + `go-fitz` |
| Markdown 输出格式 | 文本段落 → Markdown 正文 |
| 自动文件类型检测（PDF vs 图片） | 根据扩展名/魔数判断 |
| MCP 工具 `ocr_text` | 注册到 MCP Server |

### P2 - 版面分析 + Markdown 截图嵌入（~3 天）

| 任务 | 内容 |
|---|---|
| PP-DocLayout ONNX 版面检测 | `internal/layout/detector.go` |
| 区块分类与阅读排序 | text → OCR, table/figure → 截图 |
| 截图区域并保存为图片文件 | 自动裁剪版面区域 |
| Markdown 混合输出（文本 + `![table]`）| 嵌入截图引用 |
| 输出命名（input.md / ocr_TS.md）| 默认命名与冲突处理 |

### 总计

| 阶段 | 工作量 | 交付物 |
|---|---|---|
| **P0** | ~5 天 | `ocr scan image.png` 中英文文字识别可用 |
| **P1** | ~4 天 | PDF 输入 + Markdown 输出 + MCP 工具 |
| **P2** | ~3 天 | 版面分析 + 表格/图片截图嵌入 Markdown |

---

## 参考资源

| 项目 | 链接 | 用途 |
|---|---|---|
| RapidOCR | https://github.com/RapidAI/RapidOCR | ONNX 模型来源 + Python 参考实现 |
| PaddleOCR | https://github.com/PaddlePaddle/PaddleOCR | PP-StructureV3 文档 + 模型 |
| multippt/gopaddleocr | https://github.com/multippt/gopaddleocr | Go 实现参考（架构设计） |
| weihuanwan/paddleocr-go | https://github.com/weihuanwan/paddleocr-go | Go 实现参考（版面/表格） |
| RapidOCR ONNX 模型 | https://huggingface.co/SWHL/RapidOCR | 预转好 ONNX 模型 |
| go-fitz (MuPDF) | https://github.com/gen2brain/go-fitz | PDF→图片 |
| go-fitz (MuPDF) | https://github.com/gen2brain/go-fitz | PDF→图片 |
| pure-onnx | https://github.com/amikos-tech/pure-onnx | 无 CGO ONNX Runtime |

---

## 实现状态

| 模块 | 状态 | 说明 |
|---|---|---|
| `docs/guide-ocr.md` | ✅ 已定稿 | 技术选型已确认 |
| `internal/ocr/ocr.go` | ✅ 已完成 | Engine 核心类型 + ONNX 会话管理 + Scan/ScanFile |
| `internal/ocr/models.go` | ✅ 已完成 | 模型注册表（rapidocr-zh/en） |
| `internal/ocr/detect.go` | ✅ 已完成 | DBNet ONNX 推理 |
| `internal/ocr/detect_preproc.go` | ✅ 已完成 | 检测预处理（resize + ImageNet 归一化 + pad） |
| `internal/ocr/detect_postproc.go` | ✅ 已完成 | DB 二值化 + 连通域 + NMS |
| `internal/ocr/rec.go` | ✅ 已完成 | CRNN ONNX 推理 + CTC greedy 解码 + 字典映射 |
| `internal/ocr/rec_preproc.go` | ✅ 已完成 | 识别预处理（crop + resize 48px + pad） |
| `cmd/ocr.go` | ✅ 已完成 | `ocr init` + `ocr scan` 命令 |
| PDF 输入 | ⏳ 待讨论 | CGO 依赖待确认 |
| MCP 工具 `ocr_text` | ⏳ P1 | 依赖管线稳定后 |
| 英文模型（`rapidocr-en`） | ⏳ P1 | 模型 URL 已注册，需完善英文字典 |
| 版面分析（PP-DocLayout） | ⏳ 搁置 | 暂无需求 |

---

## 改进计划

> 对比 PixPin（ONNX RapidOCR）和 Umi-OCR 发现的能力差距，按收益排序。

| 优先级 | 改进项 | 状态 | 目标 |
|---|---|---|---|
| **P0** | **检测输入分辨率** | ✅ 已完成 | `DetMaxSide` 现在可配置（`SetMaxSide()`），支持动态选择输入尺寸，小图不放大 |
| **P0** | **检测框 affine 矫正** | ✅ 已完成 | 新增 `cropTextLine()`，DBNet 旋转框 affine 矫正到水平再送识别 |
| **P1** | **方向分类器接入管线** | ✅ 已完成 | `classifyDirection()` 集成到 `Recognize`，倒置文本自动旋转 180° |
| **P1** | **英文模型升级到 PP-OCRv4** | ❌ 无需操作 | PP-OCRv4 无英文识别模型，v3 即最新可用版本 |
| **P1** | **多栏版面排序 (GapTree)** | ✅ 已完成 | 移植自 Umi-OCR，已替换 `sortBoxesReadingOrder` |
| **P1** | **段落分析 (ParagraphParse)** | ❌ 待开发 | CJK 智能空格 + 自然段归并 |
| **P1** | **全局旋转估计** | ❌ 待开发 | `line_preprocessing.py` 中位数旋转角度统一矫正 |
| **P2** | **检测性能优化** | ❌ 待开发 | `draw.Draw` 或直接 RGBA 操作替代逐像素裁剪 |
| **P2** | **DP 切词触发门槛** | ❌ 待开发 | 降低触发阈值或动态判断 |
| **P2** | **词表修正与线程安全** | ❌ 待开发 | 实例字段 + 可扩展纠错 |
