# OCR 文字识别

离线文字识别（Optical Character Recognition），基于 ONNX Runtime 本地推理。
无需联网、无需 API Key，数据不出设备。

## 快速开始

```bash
# 1. 下载 OCR 模型（仅需一次，约 25 MB）
aigc-cli ocr init

# 2. 识别图片中的文字
aigc-cli ocr scan receipt.jpg      # → 输出 receipt.md
aigc-cli ocr scan document.pdf     # → 输出 document.md
```

## 命令参考

### `ocr init` — 下载模型

```bash
aigc-cli ocr init                      # 下载全部模型（中英文）
aigc-cli ocr init --list               # 列出可用模型包
aigc-cli ocr init --list-installed     # 列出已安装模型
```

模型下载到 `~/.config/aigc-cli/models/ocr/`。

### `ocr scan` — 识别文字

```bash
aigc-cli ocr scan image.png              # 输出 image.md（Markdown 格式）
aigc-cli ocr scan --lang en doc.png      # 指定语言：auto（默认），zh，en
aigc-cli ocr scan --json receipt.jpg     # JSON 输出（含坐标 + 置信度）
aigc-cli ocr scan --preview photo.jpg    # 同时打印识别结果到终端

# PDF
aigc-cli ocr scan document.pdf           # 自动判断文字 PDF / 扫描件
aigc-cli ocr scan --pages 1-3,5 doc.pdf # 指定页码范围

# 从 stdin 读取
cat document.png | aigc-cli ocr scan
```

#### 输出文件命名

```
默认: input_name.md（或 input_name.json 配合 --json）
冲突: ocr_<timestamp>.md
指定: 通过 -o <path>（当前未实现，待后续版本）
```

#### `--json` 输出格式

包含逐行文字、四点坐标和置信度：

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

## 支持格式

| 类型 | 格式 |
|---|---|
| **图片** | JPEG、PNG、GIF、BMP、WebP |
| **PDF** | 文字 PDF（直接提取文本，无需安装额外工具）<br>扫描件 PDF（需安装 `mutool`，见下文） |

## PDF 处理流程

`ocr scan` 对 PDF 自动执行双路径策略：

1. **文字 PDF**（有可选文本层）→ `ledongthuc/pdf` 直接提取文本，立即输出
2. **扫描件 PDF**（纯图片，无文本）→ `mutool` 渲染为 PNG → OCR 识别

判断标准：每页提取到 50 个以上有效字符视为文字 PDF，否则走 OCR 路径。

### 扫描件 PDF 需要 mutool

扫描件 PDF 需要 **mupdf-tools**（`mutool` 命令）来渲染页面：

```bash
macOS:  brew install mupdf-tools
Linux:  apt install mupdf-tools     # 或 pacman -S mupdf-tools
Windows: https://mupdf.com/downloads
```

也可将 `mutool` 二进制放入 `~/.config/aigc-cli/bin/`，程序会自动优先使用。

## 语言支持

`--lang` 参数控制识别使用的模型：

| 值 | 行为 |
|---|---|
| `auto`（默认）| 先中文模型识别，对英文占多数的行用英文模型重识别，取置信度高的结果 |
| `zh` | 仅中文模型，更快 |
| `en` | 仅英文模型 |

英文模型 (`rec_en_PP-OCRv3`) 在 `ocr init` 时随中文模型一起下载，`auto` 模式下自动使用。

## MCP 集成

注册了 `ocr_text` 工具，Claude Desktop / Cursor 等 MCP 客户端可直接调用：

```json
{
  "name": "ocr_text",
  "arguments": {
    "file_path": "/path/to/receipt.jpg",
    "lang": "auto",
    "output_format": "text"
  }
}
```

参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| `file_path` | string | 图片或 PDF 路径（必填） |
| `lang` | string | `auto` / `zh` / `en`，默认 `auto` |
| `output_format` | string | `text`（默认）或 `json` |

## Chat Agent 中使用

`aigc-cli chat` 交互式 REPL 中内置 `ocr_text` 工具，AI 可以在对话中直接调用 OCR 识别图片和 PDF：

```
> 帮我识别这张发票
```

Agent 会自动调用 `ocr_text` 工具并返回识别结果。

## 常见问题

**Q: 识别结果全是乱码？**
A: 确保已运行 `aigc-cli ocr init` 下载模型。如果模型已下载，尝试 `--lang` 参数指定语言。

**Q: PDF 提示 mutool not found？**
A: 扫描件 PDF 需要渲染工具。安装 `mupdf-tools` 或将 `mutool` 放入 `~/.config/aigc-cli/bin/`。文字 PDF 不需要。

**Q: 模型文件在哪？**
A: `~/.config/aigc-cli/models/ocr/`，包含检测模型、识别模型、方向分类器和字典文件。

**Q: 如何只识别英文？**
A: 用 `--lang en` 参数。

**Q: 支持表格识别吗？**
A: 当前版本输出纯文本或 JSON，不做版面分析。表格区域文字会被按行识别输出。版面分析（PP-DocLayout）在 P2 计划中。
