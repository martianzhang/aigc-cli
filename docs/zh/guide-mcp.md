# MCP Integration

`aigc-cli` implements the [Model Context Protocol](https://modelcontextprotocol.io/) (MCP), allowing AI agents to call image generation, video generation, model queries, and AIGC detection directly — without leaving your chat.

## Supported Clients

### Claude Desktop

Add to your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "aigc-cli": {
      "command": "aigc-cli",
      "args": ["mcp"]
    }
  }
}
```

Set your API key via environment variable before launching:

```bash
export OPENAI_API_KEY="sk-xxx"
export OPENAI_BASE_URL="https://api.apimart.ai"       # or OpenRouter, OpenAI, etc.
```

### Cursor

Add to Cursor's MCP configuration (`~/.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "aigc-cli": {
      "command": "aigc-cli",
      "args": ["mcp"]
    }
  }
}
```

### VS Code / Windsurf / Any MCP Host

Same config pattern — every MCP-compatible client uses the same entry:

```json
{
  "mcpServers": {
    "aigc-cli": {
      "command": "aigc-cli",
      "args": ["mcp"]
    }
  }
}
```

Ensure the binary is on your `$PATH`, or use an absolute path:

```json
{
  "mcpServers": {
    "aigc-cli": {
      "command": "/absolute/path/to/aigc-cli",
      "args": ["mcp"]
    }
  }
}
```

## Available Tools

| Tool | Description | API Key Required |
|---|---|---|
| `generate_image` | 文生图 / 图生图 / Inpainting；支持可选白名单参数 `provider` | ✅ |
| `generate_video` | 视频生成（异步提交 → 轮询）；固定使用 `defaults.video.provider`，不接受 `provider` 参数 | ✅ |
| `generate_music` | 音乐生成（APIMart suno/flowmusic 异步、OpenRouter Lyria、阿里云百炼 Fun-Music）；支持可选 `provider` | ✅ |
| `generate_speech` | 文字转语音 TTS；支持可选 `provider` / `model` / `voice` / `format` | ✅ |
| `transcribe_audio` | 语音转文字 STT（本地音频文件）；支持可选 `provider` / `model` / `language` | ✅ |
| `midjourney_imagine` | Midjourney 艺术生图（比 `generate_image` 贵 5-10 倍，一次产出 4 张变体）。仅在用户明确要求 Midjourney，或需要高度艺术化/风格化/绘画感结果时使用；一般场景请优先 `generate_image` | ✅ |
| `midjourney_describe` | 图生文（反向提示词）：上传图片 URL，返回 Midjourney 生成该图所用的提示词 | ✅ |
| `midjourney_reroll` | 重新生成（同一提示词，全新结果），需要之前的 MJ `task_id` | ✅ |
| `midjourney_video` | 图生视频：把一张图片变成短视频 | ✅ |
| `list_models` | 列出市场可用模型（可按 `type` 过滤），无需 API Key | ❌ |
| `get_model_pricing` | 查询指定模型的定价明细，无需 API Key | ❌ |
| `get_balance` | 查询 API Key 余额与账号总余额 | ✅ |
| `get_task` | 查询异步任务 / OpenRouter 视频 job 的状态与结果 | ✅ |
| `get_config` | 返回当前生效配置（密钥已脱敏），用于确认生成时应使用的参数 | ❌ |
| `caption_image` | 读取或写入图片的 caption/描述（支持 JPEG/PNG） | ❌ |
| `search_ideas` | 搜索本地灵感库（关键词，或 `random=true` 随机返回） | ❌ |
| `remove_background` | 离线抠图（RMBG 2.0 语义分割），可选 `replace_color` / `replace_image` / `autocrop` | ❌ |
| `convert_depth` | 图片/视频 → 灰度深度图（Depth Anything V2，本地 ONNX）；`annotate` 可叠加骨架（人体姿态）或人脸（关键点+眼睛）标注 | ❌ |
| `detect_image` | 检测 C2PA / SynthID / TC260 / EXIF 等 AIGC 信号（完全离线） | ❌ |
| `remove_watermark` | 检测并移除可见 AI 水印（内置 gemini，其他厂商需 `learn-watermark` 学习），输出 `<原图>_clean<ext>` | ❌ |
| `add_watermark` | 添加可见 AI 水印（仅用于构造去水印测试样本；内置 gemini alpha map，未知名称按文字渲染） | ❌ |
| `crop_watermark` | 裁切去水印（无需学习模板；`target` 支持 auto / n% / WxH） | ❌ |
| `recognize_text` | 离线 OCR 识别图片/PDF 文字（需先 `aigc-cli ocr init`；输出 markdown 或 json） | ❌ |
| `kb_find` | 本地知识库关键词 + 语义搜索 | ❌ |
| `kb_search` | 联网搜索（`provider` 选择搜索引擎 duckduckgo/firecrawl）并把结果入库 | ❌ |
| `kb_add` | 把本地文件加入知识库（.md/.txt/.go/.py/.json/.yaml/.html） | ❌ |
| `kb_fetch` | 抓取 URL 正文转 Markdown 后入库 | ❌ |
| `kb_list` | 列出知识库中的全部文档 | ❌ |
| `kb_show` | 按 ID（前 12 位）查看知识库文档全文 | ❌ |

> 所有工具都声明了 MCP annotations（`readOnlyHint` / `destructiveHint` / `idempotentHint` / `openWorldHint`）：宿主对只读工具可跳过确认，本服务器的任何工具都不是破坏性的。
>
> 声明为只读（`readOnlyHint=true`，宿主可跳过确认）的工具：`list_models`、`get_model_pricing`、`get_balance`、`get_task`、`get_config`、`caption_image`、`search_ideas`、`detect_image`、`recognize_text`、`kb_find`、`kb_list`、`kb_show`。

> `generate_image` / `generate_video` / `generate_music` 与对应的 CLI 命令共用同一套 Provider 路由，所有已支持的 Provider（OpenRouter、APIMart、Agnes、OpenLux、ModelScope、Gemini、ZeekAI、阿里云百炼等）无需额外配置即可使用。
>
> `midjourney_*` 工具通过 `defaults.midjourney.provider`（或全局 `api_key`/`base_url`）解析 Provider，`midjourney_imagine` 会合并 `defaults.midjourney.*` 默认参数。
>
> `generate_image` / `generate_music` / `generate_speech` / `transcribe_audio` 与 4 个 `midjourney_*` 工具还支持可选的 `provider` 参数（仅接受 `config.providers` 中的名称白名单）；`generate_video` 不支持该参数，固定使用 `defaults.video.provider`。详见下文「按调用指定 Provider」。
>
> 图片参数以配置为准：`defaults.image.*` 会覆盖 Agent 传入的同名参数，除非设置 `defaults.chat.allow_tool_override: true`。
>
> 产出媒体文件的工具（`generate_image`、`generate_speech`、`convert_depth`、`remove_background`、`remove_watermark`、`add_watermark`、`crop_watermark`）会在原有文本结果之外附加**内联的图片/音频内容块**（MCP image/audio content），Claude Desktop 等宿主可直接在对话中渲染生成结果；视频等 MCP 尚不支持的媒体仍只返回文件路径。单次结果最多内联 4 个文件、每个不超过 4 MiB，超出部分仅在文本中提示；设置环境变量 `AIGC_MCP_EMBED_MEDIA=0`（或 `false`/`off`）可关闭内联，只返回文本路径。
>
> 异步任务工具 `generate_video` / `generate_music` 在宿主传入 `progressToken` 时（如 Cursor）会发送**粗粒度进度通知**（0.05 提交前、0.40 进入提交→轮询阶段），便于宿主显示进度指示；目前只发送这两个里程碑，逐次轮询的百分比更新暂未提供。未传 `progressToken` 时完全静默。

### 按调用指定 Provider（白名单）

`generate_image` / `generate_music` / `generate_speech` / `transcribe_audio` 以及 4 个 `midjourney_*` 工具都支持可选参数 `provider`：

- **留空（默认）**：行为与不传该参数完全一致，使用 `defaults.{命令}.provider` 指定的 Provider（未配置时回退全局 `api_key` / `base_url`）。
- **传值时**：只能是 `config.providers` 中已配置的 **Provider 名称**（白名单）。URL、路径，以及未在配置中定义的名称一律被拒绝并返回错误。

> `generate_video` **不支持** `provider` 参数：它始终使用 `defaults.video.provider`（未配置时回退全局 `api_key` / `base_url`）。

```yaml
# ~/.config/aigc-cli/config.yaml
providers:
  my-openrouter:
    type: openai
    api_key: sk-or-xxx
    base_url: https://openrouter.ai/api/v1

defaults:
  image:
    provider: my-openrouter
```

```json
{ "prompt": "a cat under the stars", "provider": "my-openrouter" }
```

**安全性**：MCP 工具**不接受** `base_url` 或 `api_key` 参数，也不提供任何写配置的工具（没有 `set_config`）。Agent 只能引用你在 `config.yaml` 中预先配置并信任的 Provider 名称，无法把请求指向任意地址，也无法注入凭证。

### `model` 参数的真实行为

- `generate_image` / `generate_video`：**配置优先** —— `defaults.image.model` / `defaults.video.model` 会覆盖 Agent 传入的 `model`，除非设置 `defaults.chat.allow_tool_override: true`（此时 Agent 传值优先，配置只在缺省时补齐）。
- `generate_music` / `generate_speech` / `transcribe_audio`：Agent 传入的 `model` **按原样使用**；仅当未传时才回退到 `defaults.music.model` / `defaults.audio.speak_model` / `defaults.audio.transcribe_model`（再回退代码默认值）。

### Filter Tools

Use `tools_enable` / `tools_disable` in `config.yaml` to restrict which tools are available in MCP mode:

```yaml
tools_enable:                        # only allow image generation and model queries
  - "generate_*"
  - "list_models"
  - "get_model_pricing"

tools_disable:                       # additionally exclude specific tools
  - "remove_background"
```

Glob patterns are supported (`*` matches any tool name). Empty or absent lists = all tools allowed. These settings affect both MCP and chat tools.

### 工具面决策（设计说明）

`web_fetch`、`grep`、`read_file`、`find` 是**仅限 Chat** 的 Agent 工具（位于 `internal/cli/chat`，供交互式 REPL / Agent Loop 使用），**刻意不通过 MCP 暴露**：它们会赋予 Agent 不受沙箱限制的任意文件与网页访问能力，一旦模型读到的页面或文件里藏有提示词注入（prompt injection），就可能被诱导读取并外泄本地文件；MCP 工具面被有意限定在 AIGC 能力范围内（生成、检测、OCR、知识库、Provider/配置查看）。这是刻意的边界，不是遗漏。

### 安全加固（v3.3.0）

发版前安全审计（攻击面测绘 + 3 条线 hunter + 2 名 PoC 工程师独立复现）后，MCP 面新增以下防线：

- **`output_path` 约束**：watermark / background / depth 工具的显式输出路径必须落在 `output_dir`（`cfg.Output`）或**输入文件所在目录**两个根之内；符号链接目标被拒绝（防穿透截断）。不传 `output_path` 时默认「输出到输入旁」行为不变。
- **本地图片校验**：`image_urls` 引用的本地文件必须是**可解码的图片**（≤32 MiB），非图片文件（如 `~/.ssh/id_rsa`、`config.yaml`）不会被内联发送给 Provider。
- **解码炸弹防护**：本地图片处理工具先读头部尺寸，>1 亿像素（100 MP）拒绝解码。
- **`generate_speech.format` 枚举**：仅 `mp3/wav/opus/aac/flac/pcm`，阻断 `speech_<ts>.<ext>` 路径穿越。
- **任务 ID 清洗**：`get_task` 下载文件名使用 `filepath.Base` 后的安全令牌，阻断 `task_id` 目录穿越。
- **KB 路径修复**：`kb_*` 工具指向 `~/.config/aigc-cli/knowledge`（与 CLI/chat 一致），消除「字面 `~` 目录 + 预置知识库投毒」向量。
- **KB 网络出口**：`kb_fetch` / `kb_search` 走全局 HTTP 客户端（尊重 `http_proxy`），且**拒绝回环/内网/链路本地/元数据地址**（含重定向后复检），响应体读取有上限。
- 修复 MCP server 在配置缺少 `defaults:` 段时的启动 panic。

## Prompts / 工作流模板

MCP Prompts 是宿主（Claude Desktop、Cursor 等）里可直接点击调用的**工作流模板**：选中后会带着预设的指令和参数，引导 Agent 调用对应的 MCP 工具完成整条流程。

| Prompt | 参数 | 说明 |
|---|---|---|
| `generate_product_shot` | `product`（必填）、`style`（可选）、`aspect_ratio`（可选） | 生成商业产品图：调用 `generate_image`，使用影棚布光、干净无缝背景、商业广告质感，并按需应用 style / 宽高比 |
| `detect_ai_image` | `file_path`（必填） | 检测图片是否为 AI 生成：调用 `detect_image`，解读 C2PA / TC260 / SynthID / FFT / ONNX / 可见水印等融合信号并给出明确结论 |
| `image_to_video` | `image_url`（必填）、`prompt`（可选） | 图生视频：调用 `generate_video`，以该图片为参考、可选运动提示词，完成后报告保存的文件路径 |
| `remove_image_background` | `file_path`（必填）、`replace_color`（可选） | 离线抠图：调用 `remove_background`，仅在提供 `replace_color` 时替换背景色，完成后报告输出路径 |

必填参数缺失时，Prompt 仍会返回模板，并在消息中提示 Agent 先向用户询问缺失的值。

列出所有已注册的 Prompt（不启动服务器）：

```bash
aigc-cli mcp --list-prompts
```

## Resources / 资源

MCP Resources 是宿主可以直接读取的**只读数据源**（与工具不同，不会产生任何副作用），可在 Claude Desktop、Cursor 等宿主中通过 `@` / 附件方式引用。

| Resource URI | 类型 | 内容 |
|---|---|---|
| `aigc://providers` | 静态 | 已配置的 provider 名称与 host（`config.providers`），用于给工具的 `provider` 参数挑选合法取值；**不包含任何 API Key** |
| `aigc://config` | 静态 | 当前生效配置，所有密钥已脱敏（与 `--print-config` / `get_config` 工具相同的脱敏保证） |
| `aigc://output` | 静态 | 输出目录的顶层文件列表（文件名、大小、修改时间），非递归，最多 200 条并标注截断 |
| `aigc://output/{filename}` | 模板 | 读取输出目录中的单个文件：图片/音频返回 base64 blob，`.txt/.md/.json/.yaml/.yml/.csv/.log` 返回文本 |

安全约束（全部只读）：

- 仅允许读取输出目录（`--output` / `output_dir`）**顶层**的普通文件；`../`、`/`、`\`、绝对路径、卷名、符号链接一律拒绝。
- 单个文件超过 4 MiB 拒绝读取；不支持的文件类型返回明确错误。
- 未配置输出目录时，`aigc://output` 返回提示，模板资源报错。
- 任何资源都不会暴露 API Key 或其他凭据。

## Configuration

MCP mode reuses the existing config system with three options:

```bash
# Option 1: Config file
# ~/.config/aigc-cli/config.yaml

# Option 2: Environment variables
OPENAI_API_KEY=sk-xxx aigc-cli mcp

# Option 3: CLI flags
aigc-cli mcp --api-key sk-xxx --output ./downloads
```

### CLI Flags

| Flag | Description |
|---|---|
| `--api-key` | API key (overrides config/env) |
| `--base-url` | API base URL |
| `--http-proxy` | HTTP proxy URL |
| `--output` | Download directory for generated files |
| `--list-tools` | List registered MCP tools and exit |
| `--list-prompts` | List registered MCP prompts and exit |

## Test Available Tools

You can list all registered MCP tools with a simple flag:

```bash
aigc-cli mcp --list-tools
```

This respects `tools_enable`/`tools_disable` config — only the tools that would actually be registered are shown.

Prompts can be listed the same way:

```bash
aigc-cli mcp --list-prompts
```

For the full tool schema (JSON-RPC `tools/list`), send via stdio:

```bash
printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}\n{"jsonrpc":"2.0","id":2,"method":"notifications/initialized"}\n{"jsonrpc":"2.0","id":3,"method":"tools/list"}\n' | aigc-cli mcp | jq -r '.result.tools[]?.name // empty'
```

Or pipe to `jq .` for the full formatted JSON of each response message.

## Dynamic Tool Descriptions

On startup, `aigc-cli mcp` reads your `config.yaml` defaults (model, size, resolution, quality, etc.) and injects them into each tool's description. The AI agent sees your current configuration and only overrides parameters when the user explicitly requests it.
