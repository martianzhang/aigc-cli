# Preview

使用 `aigc-cli preview`（别名 `pr`）查看图片、Markdown/文本文件、读取元数据并生成图片描述。

## 用法

```bash
# 在终端中查看图片
aigc-cli preview photo.png

# 查看详细元数据
aigc-cli preview photo.png --detail

# 生成图片描述
aigc-cli preview photo.png --describe

# 预览 Markdown 文档（终端内渲染）
aigc-cli preview ocr_notes.md

# 预览纯文本（超长自动分页）
aigc-cli preview notes.txt
```

## Markdown 预览

`preview` 复用 chat TUI 的 Markdown 渲染器（同一套 glamour 样式、表格与换行处理），把 `.md` / `.markdown` 文件渲染成终端样式（标题、列表、表格、代码块）：

- 只有 **stdout 是真实终端** 时才渲染。管道、重定向（`| grep`、`> notes.txt`、日志）保持原始 Markdown 源码不变，方便继续处理。
- 加 `--raw` 可以在终端里强制输出源码。
- 宽度跟随终端列数，过窄时按最小 20 列排版。
- 设置 `NO_COLOR` 会关闭颜色（仍保留排版）。
- 用法示例：

```bash
# 终端内渲染
aigc-cli preview ocr_notes.md

# 多个文档依次渲染
aigc-cli pr docs/*.md

# 保持源码输出（管道场景通常不需要加）
aigc-cli preview notes.md --raw | grep TODO
```

## Markdown 图片

终端支持内联图片时（iTerm2 / Kitty / Sixel），文档引用的图片会就地显示，并在图片下方显示 alt 文本（方括号里的文字；没有 alt 就不显示图注）：

- 支持 `![alt](path)`、`![alt](path "title")`、`![alt][id]` 引用式图片，以及 `<img src="...">`。
- 相对路径按 Markdown 文件所在目录解析；绝对路径、`file://`、`http(s)://`、`data:` 都可使用。
- 远程和 `data:` 图片会先下载到临时文件，预览后立即删除。
- 终端不支持内联图片、或设置了 `--no-inline-preview` / `AIGC_CLI_NO_INLINE_PREVIEW` 时，不会下载远程图片，图片引用保留为文本。
- 代码块、行内代码和链接里的图片不会被抽出渲染，避免破坏原有结构。

## 分页

Markdown 和纯文本（`.txt` / `.text` / `.log`）在终端里超过一屏时，会像 `bat` 一样自动进入分页器：

- 优先使用 `$PAGER`；未设置时用 `less -R`（保留颜色），再退到 `more`。
- 上下键/`j`/`k` 滚动，空格翻页，`q` 退出；退出后内容仍保留在终端回滚区（less 使用 `LESS=FRX`，不切备用屏）。
- 加 `--no-pager` 或设置 `PAGER=cat` 可直接输出；管道、重定向本来就不会分页。
- 带内联图片的 Markdown 不分页：图片转义序列经过分页器会错位。这类文档想分页可加 `--no-inline-preview`（图片引用会保留为文本）。

## 终端内联显示

在 iTerm2 和支持 Sixel 的终端中，`preview` 会把图片直接渲染在终端里，而不是交给外部应用：

- 只有 **stdout 是真实终端** 时才会内联渲染。管道、重定向或捕获输出（`| tee`、日志文件、IDE/TUI 终端）会把转义序列当作普通文本打印——你看到的 base64 内容其实就是图片本身。
- 内联载荷不是原图：超过预算的图片会先缩放并重新编码（JPEG；带透明通道时用 PNG），所以几 MB 的 PNG 预览通常只有几十 KB。
- 内联载荷上限为 512 KB。仍然超限的图片（例如需要保留动画而原样发送的 GIF）会改用系统默认应用打开。
- 设置 `AIGC_CLI_NO_INLINE_PREVIEW`（非空）或全局参数 `--no-inline-preview` 可完全跳过内联转义序列。
- 超过预算的图片会缩放到终端宽高的约 60%，不会占满整个窗口。
- 已经小于预算的图片按原始尺寸显示——预览不会放大。
- 保持宽高比，图片完整可见，不会滚出屏幕。
- 尺寸使用命令运行时的终端真实大小（列 × 行），请先调整好窗口再运行 `preview`。
- 无法检测终端大小时（没有 TTY 或终端未上报），按保守的 80×24 计算，避免大图按原始尺寸输出。
- 终端不支持内联图片时，改用系统默认应用打开。

## 元数据显示

`--detail` 会显示 EXIF 元数据，包括：

- 相机厂商/型号
- 拍摄时间
- GPS 坐标
- 图片尺寸
- 文件大小
- 色彩配置

## 图片描述

`--describe` 会使用 AI 视觉（如果已配置）或 EXIF 数据生成图片描述。
