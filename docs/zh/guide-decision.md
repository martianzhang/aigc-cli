# 决策模型指南

`aigc-cli decision`（别名 `decide`、`systemone`）对一段文本运行"决策类"问题（单选 / 是非 / 评分），并返回带概率的结构化答案。本命令**不**生成自然语言文本。响应为 JSON，可直接管道给 `jq` 得到可读视图（见[输出](#输出)）。

## 命令

```
aigc-cli decision [flags]
```

## 参数

| 参数 | 说明 |
|---|---|
| `--json <path\|inline-json\|->` | 题库（question bank）：JSON 或 JSONC 文件。支持文件路径、内联 JSON 字符串、`-` 读 stdin。缺省回退到 `defaults.decision.bank` |
| `--questions a,b,c` | 仅回答指定的问题（ASCII 逗号分隔，可重复；题目名可用中文）。省略则回答题库中**所有**问题 |
| `--state <text\|path\|->` | 待判定的素材。支持直接文本、文件路径、`-` 读 stdin。若内容是合法 JSON 对象/数组，按结构化 state 发送；否则按纯字符串发送 |
| `--image <path\|data-uri>` | 附加图片（PNG/JPEG/WebP），与 state 一起联合评分；可重复。支持本地文件或 base64 data URI。需要 Ollama >= 0.35.1 的 Clef / Clef-Flash |
| `--audio <path\|data-uri>` | 附加音频（WAV/MP3/M4A/AAC/OGG/OPUS/FLAC），与 state 一起联合评分；可重复。支持本地文件或 base64 data URI。需要 Clef-Omni / 支持音频的服务端 |
| `--video <path\|data-uri>` | 附加视频（MP4/MOV/WebM/MKV/AVI），服务端按 2 fps 抽帧并附带音轨；可重复。支持本地文件或 base64 data URI。需要 Clef-Omni / 支持视频的服务端 |
| `--image-resize <px>` | 发送前把附加图片的最长边缩放到不超过 N 像素。默认 `1024`；`0` 保留原图。真正降低耗时的是它——见[多模态图片](#多模态图片) |
| `--list` | 列出题库中所有问题的名称后退出 |
| `--dry-run` | 打印等价的 curl 请求，不真正调用 API |
| `-P, --provider` | （全局）切换命名 provider，例如 `ollama` 或 `openrouter` |
| `-m, --model` | （全局）切换模型，例如 `tev1`、`tev1:0.8b`、`nimble`、`typesafe/jev-latest` |

## 题库 JSON

题库文件是 JSON 或 JSONC（允许 `//` 注释和尾逗号）。顶层字段：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `model` | string | 否 | 覆盖默认模型 |
| `state` | string / object / array | 否 | 默认 state，可被 `--state` 覆盖 |
| `images` | 字符串数组 | 否 | 默认图片（本地路径或 base64 data URI），可被 `--image` 覆盖 |
| `audio` | 字符串数组 | 否 | 默认音频（本地路径或 base64 data URI），可被 `--audio` 覆盖 |
| `videos` | 字符串数组 | 否 | 默认视频（本地路径或 base64 data URI），可被 `--video` 覆盖 |
| `questions` | object | 是 | 命名问题字典，1–64 个问题 |

每个问题字段：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | `choice` / `noul` / `score` | 是 | 问题类型 |
| `instructions` | string | 是 | 问题正文 |
| `criteria` | object / array | 视类型而定 | `choice`：`{ 选项: 描述 }`；`score`：`[低→高, …]`（从低到高）；`noul`：可省略 |

完整示例见 [examples/decision/triage.json](examples/decision/triage.json)，配套素材见 [examples/decision/state.txt](examples/decision/state.txt)。

### 三类问题与返回

| 类型 | 返回字段 |
|---|---|
| `choice` | `choice`（选中的选项 key）、`probabilities`（每选项概率）、`confidence` |
| `noul` | `noul`（答案为"是"的概率，0–1）。**没有** `confidence` 字段 |
| `score` | `score`（按概率求和得到的分档，可能为小数）、`legend`（索引→标签）、`probabilities`、`confidence` |

> `confidence` 衡量概率分布的集中程度，**不是**答案正确的概率。

### 多模态图片

决策模型（Clef / Clef-Flash，Ollama >= 0.35.1）还可以判定图片。用 `--image` 附加（可重复）；所有图片由请求中**全部**问题共享，与文本 `state` 一起联合评分：

```bash
aigc-cli decision -m clef-flash --image form.png --questions complete,type --state "The agent wants to submit the attached form."
```

图片按请求顺序发送。本地 PNG/JPEG/WebP 文件自动编码；base64 `data:` URI 会剥离为 raw base64。

#### 耗时与 `--image-resize`

决策模型的耗时由**像素尺寸决定，而非文件大小**——图片会被转成视觉 token，模型在一次前向里对全部内容打分。**重新压缩（降质量 / 减体积）没有用**，只有缩放像素才有效：

| 输入 | input_tokens | 耗时 |
|---|---|---|
| 纯文本 | 164 | ~1s |
| 2848×1600 PNG（2.4 MB） | ~4100 | ~47s |
| 同尺寸重编码 q75/q40（像素不变） | ~4100 | ~47s |
| **2848×1600 缩到 1024px** | ~800 | ~6s |
| 缩到 768px | ~570 | ~4s |

`--image-resize`（默认 `1024`）在发送前按最长边缩放，保持宽高比与格式，且**永不放大**。传 `--image-resize 0` 发送原图；需要更多细节时传更大的值（如 `1600`）。

```bash
# 大截图会自动缩到 1024px → 明显更快
aigc-cli decision -m clef-flash --image screenshot.png --state "请判定所附截图。"
aigc-cli decision -m clef-flash --image screenshot.png --image-resize 768 --state "请判定所附截图。"
```

### 音频与视频（多模态）

Clef-Omni / Clef 家族还可以判定音频与视频。用 `--audio` 和 `--video` 附加（均可重复）；每个片段由请求中**全部**问题共享，与文本 `state` 及图片一起联合评分：

```bash
aigc-cli decision -m clef-omni --audio call.wav --questions escalation --state "请审核所附通话录音。"
aigc-cli decision -m clef-omni --video dashcam.mp4 --questions collision --state "片段里是否发生了碰撞？"
```

与图片（发 raw base64）不同，音频和视频以 **base64 data URL** 发送（`data:audio/wav;base64,...`、`data:video/mp4;base64,...`），让 MIME 类型随字节一起传递。本地文件按扩展名判定类型：音频支持 `.wav .mp3 .m4a .aac .ogg .oga .opus .flac .webm`，视频支持 `.mp4 .m4v .mov .webm .mkv .avi`（每个不超过 64 MiB）。`data:` URI 原样透传；`http(s)` URL **直接拒绝**——请先下载。视频在服务端按 **2 fps** 抽帧；当请求中每个视频都带音轨时，音轨会被一并识别。

> 音频/视频需要支持它们的模型服务端。在 OpenRouter 上，`cloudflare/clef-omni` **目前只接受文本与图片**——音频/视频输入标注为 "coming soon"；HuggingFace 上发布的 `Cloudflare/clef-omni` 自托管时才三者全支持。

## 输出

响应为 JSON（`model` / `answers` / `usage`），可直接管道给 `jq`：

```bash
# 取某题选中的选项
# 注意：题库用中文键时，jq 必须用方括号取值；.answers.意图.choice 会报语法错误
aigc-cli decision --json triage.json --state state.txt | jq -r '.["answers"]["意图"].choice'

# 每题一行（键无关，中英文题库通用；三个字段互斥，用 // 依次回退）
aigc-cli decision --json triage.json --state state.txt \
  | jq -r '.answers | to_entries[] | "\(.key): \(.value.choice // .value.noul // .value.score)"'

# 只在模型确信时采用，否则升级人工
aigc-cli decision --json triage.json --state state.txt \
  | jq -r 'if .answers["意图"].confidence > 0.85 then .answers["意图"].choice else "ESCALATE" end'
```

## 支持的 Provider

不同 provider 的端点路径**不同**。本项目**仅端到端验证** Ollama 与 OpenRouter；其余按官方文档记录但**本项目未测试**。

| Provider | 官方文档 | Configured `base_url` | 完整端点 |
|---|---|---|---|
| TypeSafe AI | https://docs.typesafe.ai/api | https://api.typesafe.ai | POST https://api.typesafe.ai/v1/systemone |
| OpenRouter | https://openrouter.ai/docs/guides/community/typesafe-sdk | https://openrouter.ai/api/v1 | POST https://openrouter.ai/api/v1/systemone |
| Ollama (>= 0.35.0；图片需 >= 0.35.1) | https://ollama.com/library/tev1 | http://localhost:11434 | POST http://localhost:11434/v1/systemone |
| LLM Gateway | https://docs.llmgateway.io/features/system-one | https://api.llmgateway.io/v1 | POST https://api.llmgateway.io/v1/systemone |
| LiteLLM 代理 | https://docs.litellm.ai/docs/pass_through/typesafe | `{proxy}/typesafe` | POST `{proxy}/typesafe/v1/systemone` |

- **Ollama** 无需 API Key，本地运行。
- **TypeSafe AI** 与 **OpenRouter**（以及其他）需要 API Key。

## 优先级

```
CLI 参数 > JSON（题库） > defaults.decision YAML > 代码默认值
```

`--json` 与其他 flag 同时出现时：每个**显式设置**的 flag 覆盖题库对应字段；未触及的字段保持题库原值。仅传 `--json` 不带任何 flag 时，题库**整体发送**。`--image` / `--audio` / `--video` 覆盖题库的 `images` / `audio` / `videos` 数组。

## 示例

> 以下示例假设已配置 `defaults.decision.provider`（如 `ollama`）。若未配置，请加 `-P ollama`（本地）或 `-P openrouter`。

```bash
# 本地 Ollama（免费、离线）
aigc-cli decision --json triage.json --questions 意图,退款 --state state.txt

# 切换到 OpenRouter 上的远程 Jev 模型
aigc-cli decision -P openrouter -m typesafe/jev-latest --questions 意图 --state state.txt

# 更小的本地模型（用 config 里的默认题库）
aigc-cli decision -m tev1:0.8b --state state.txt

# 结构化 state（对象）—— 标注上下文段落
aigc-cli decision --questions 退款 \
  --state '{"policy":"退款需在 30 天内提出。","request":"我 12 天前买的，想退款。"}'

# stdin、列清单、dry-run
cat ticket.txt | aigc-cli decision --questions 意图
aigc-cli decision --list
aigc-cli decision --questions 意图 --state state.txt --dry-run
```

## 注意事项

- `choice` / `score` 接受 2–26 个选项；tev1 训练时覆盖 2–24，**请保持在 2–24**
- 单次请求 1–64 个问题，请求体 ≤ 64 KiB
- tev1 上下文约 **2000 tokens**，请保持 state 简短
- 图片需要 **Clef / Clef-Flash**（Ollama >= 0.35.1）；纯文本模型会拒绝图片。API 只接受 **raw base64**：本地文件会自动编码，`data:` URI 会剥离/转换，`http(s)` URL **直接拒绝**——请先下载。坏图片文件（损坏、非图片、超过 32 MiB）在请求发出前即被拒绝。
- 图片耗时由**像素尺寸决定，而非文件字节**——只有 `--image-resize`（默认 1024px）能提速。重新压缩（降质量 / 减体积）**不会**减少 token。传 `--image-resize 0` 发送原图。
- 音频/视频需要 **Clef-Omni / Clef** 且服务端支持。OpenRouter 上的 `cloudflare/clef-omni` **目前只有文本 + 图片**（音频/视频 "coming soon"），因此 `--audio` / `--video` 只在自托管服务端、或 provider 开放后才可用。片段以 base64 **data URL** 发送（按扩展名判定类型，每个 ≤ 64 MiB）；`http(s)` URL 直接拒绝。
- 若没有合适选项，**务必**添加 `none` / `other` 选项——模型不会自动说"以上皆非"
- tev1 基模为 **Qwen3.5**（`ollama show tev1` → `arch qwen35`），中文题库 / 中文 state 实测可用（见 [examples/decision/triage.json](examples/decision/triage.json)）；但厂商评测以英文为主，非英文**无质量保证**——生产上请配合 `confidence` 阈值与人工兜底
- prompt injection 在厂商测试中覆盖有限：state 要当**数据**看，不要让它左右下游动作
- 决策模型可能出错——**不要**把它当作高风险决策的唯一依据
- 本命令**尚未**接入 MCP

## Config

```yaml
defaults:
  decision:
    provider: ollama            # config.providers 里定义的命名 provider（ollama / openrouter / ...）
    model: tev1                 # tev1 | tev1:0.8b | nimble | typesafe/jev-latest | cloudflare/clef-omni
    bank: ~/exams/triage.json   # 默认题库（--json 缺省时使用）
```