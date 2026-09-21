# 音乐生成

`aigc-cli music` 用自然语言提示词生成音乐。根据 Provider 自动选择后端：

- **APIMart（默认）**：异步任务模型 —— 提交任务 → 轮询 → 下载。`--model suno`（默认）或 `--model flowmusic`。
- **OpenRouter**：同步流式，`--model google/lyria-3-clip-preview`（默认，约 30 秒，mp3）或 `google/lyria-3-pro-preview`（数分钟，mp3/wav）。音频在一次调用中直接返回，**没有 task_id**。
- **阿里云百炼（Fun-Music）**：同步，一次调用返回音频 URL（24 小时有效），**没有 task_id**。`--model fun-music-v1`（默认）或 `fun-music-preview`。

## 命令

```
aigc-cli music
├── generate / gen  提交音乐生成任务
└── query <task-id> 查询任务状态并下载结果
```

## 基本用法

```bash
# APIMart suno（默认）：根据描述生成
aigc-cli music generate --prompt "city pop"

# 别名 gen
aigc-cli music gen --prompt "city pop"

# flowmusic 后端
aigc-cli music gen --prompt "rock" --model flowmusic

# 自定义歌词（suno 进入 custom 模式）+ 曲名
aigc-cli music gen --prompt "pop" --lyrics "la la la" --title "My Song"

# 纯音乐（无人声）
aigc-cli music gen --prompt "lofi beats" --instrumental

# 指定时长与格式（suno）
aigc-cli music gen --prompt "epic orchestral" --duration 120 --format mp3

# 阿里云百炼 Fun-Music（同步）
aigc-cli music gen --provider dashscope --model fun-music-v1 --prompt "夏日清新民谣"
aigc-cli music gen --provider dashscope --json '{"model":"fun-music-v1","input":{"prompt":"摇滚","gender":"male"}}'

# 查询任务：完成后自动下载音频（仅 APIMart）
aigc-cli music query task_xxx
```

---

## Provider 差异

| Provider | 模型 | 模式 | 请求字段 |
|---|---|---|---|
| APIMart（默认） | `suno`（默认） | 异步：提交 → 轮询 → 下载 | prompt / style / title / lyrics / instrumental / duration / format |
| APIMart | `flowmusic` | 异步：提交 → 轮询 → 下载 | sound_prompt / lyrics / title / length |
| OpenRouter | `google/lyria-3-clip-preview`（默认） | 同步流式，一次调用返回音频 | 约 30 秒，mp3 |
| OpenRouter | `google/lyria-3-pro-preview` | 同步流式 | 数分钟，mp3 / wav |
| 阿里云百炼 | `fun-music-v1`（默认）/ `fun-music-preview` | 同步，一次调用返回音频 URL | prompt / lyrics / instrumental / format / gender |

> `--model` 名称中包含 `flowmusic` 即走 flowmusic 后端，否则走 suno。该规则仅适用于 APIMart。
> Provider 由 base URL 自动识别：`*.maas.aliyuncs.com` 或 `dashscope.aliyuncs.com` → 阿里云百炼。

### APIMart（异步）

提交后返回 `task_id`，CLI 自动轮询直到完成并下载。`music query <task-id>` 可随时查看进度、或对历史任务重新拉取下载。

字段映射：

- **suno**
  - `--prompt`（或 `--style`）→ `prompt`（无歌词时）或 `style`（有歌词时）
  - `--lyrics` 非空 → `custom: true`，歌词写入 `prompt`
  - `--title` → `title`
  - `--instrumental` → `instrumental`
  - `--duration` → `duration`
  - `--format` → `audio_format`
- **flowmusic**
  - `--prompt`（或 `--style`）→ `sound_prompt`
  - `--lyrics` → `lyrics`（`--instrumental` 时不发送）
  - `--title` → `title`
  - `--duration` → `length`（未指定时默认 120 秒）

> suno 与 flowmusic 至少需要一个有效输入：suno 需要 `prompt`（即 `--prompt`/`--style`/`--lyrics`），flowmusic 需要 `sound_prompt` 或 `lyrics`。

### OpenRouter（同步流式）

无需配置命名 Provider，直接通过环境变量或 `--api-base` 指向 OpenRouter 即可：

```bash
export OPENAI_API_KEY="sk-or-xxx"
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"

# 默认模型（约 30 秒，mp3）
aigc-cli music gen --prompt "ambient"

# pro 模型（数分钟，可输出 mp3 / wav）
aigc-cli music gen --model google/lyria-3-pro-preview --prompt "cinematic" --format wav
```

也可以引用配置中的命名 Provider：

```bash
aigc-cli music gen --provider openrouter --prompt "ambient"
```

说明：

- 需要 OpenRouter API Key，base URL 为 `https://openrouter.ai/api/v1`。
- 同步返回，**无 task_id**，不需要（也不支持）`music query`。
- OpenRouter 没有 lyrics / instrumental / duration 请求字段，这三者会被折叠进 prompt 文本：instrumental 前置 `[Instrumental] `，歌词追加 `Lyrics:` 段，时长追加 `Target duration: about N seconds.`。

### 阿里云百炼 Fun-Music（同步）

厂商文档：<https://docs.bailian.console.aliyun.com/zh/model-studio/fun-music>

```bash
aigc-cli music gen --provider dashscope --model fun-music-v1 --prompt "夏日清新民谣"
```

说明：

- **端点**：`POST https://{WorkspaceId}.cn-beijing.maas.aliyuncs.com/api/v1/services/audio/music/generation`。
  这是 DashScope **原生**路径，**不是** chat 用的 `compatible-mode/v1`。CLI 只取 base URL 的 scheme + host 再拼原生路径，因此可直接复用为 chat 配置的百炼 Provider。
- 鉴权：`Authorization: Bearer <DASHSCOPE_API_KEY>`。
- **同步返回**：一次调用直接返回音频 URL（24 小时有效），**没有 task_id**，不需要（也不支持）`music query`。
- 模型差异：
  - `fun-music-v1`：`prompt` 与 `lyrics` 至少传一个；支持 `gender`（男/女声）。
  - `fun-music-preview`：`prompt` 必填；不支持 `gender`。
- 字段映射：`--prompt`（或 `--style`，同时给出时用 `，` 连接）→ `input.prompt`；`--lyrics` → `input.lyrics`；`--instrumental` → `input.is_instrumental`；`--format` → `input.format`。
- **无对应字段**：`--duration` 与 `--title` 会被忽略（时长由歌词长度决定），CLI 会打印告警。
- vendor 专属字段（`gender`、`enable_aigc_watermark` 等）通过 `--json` 传入；`--json` 不带参数时逐字节原样发送，需自行写完整的 DashScope 原生形状（含 `input` 嵌套）；与 CLI 参数同时出现时，显式参数覆盖 `input.*` 等对应键。
- `is_instrumental=true` 时 `lyrics` 与 `gender` 无效，CLI 会自动移除这两个字段。
- 区域/开通：该模型目前为**邀测**，仅**华北 2（北京）**可用，需在百炼模型广场申请开通。

---

## 参数

| 参数 | 短参 | 说明 |
|---|---|---|
| `--prompt` | `-p` | 音乐描述 / 风格（suno 无歌词时作为 prompt，flowmusic 作为 sound_prompt） |
| `--model` | `-m` | 后端模型：`suno`（默认）/ `flowmusic` / `fun-music-v1` / `fun-music-preview`；OpenRouter 下为模型 ID |
| `--style` | | 风格（suno 的 style 字段；prompt 为空时兜底；百炼下与 prompt 拼接） |
| `--title` | | 曲目名称（百炼不支持，忽略） |
| `--lyrics` | | 歌词（suno 非空时进入 custom 模式；flowmusic / 百炼作为 lyrics） |
| `--instrumental` | | 纯音乐（无人声） |
| `--duration` | `-d` | 时长（秒）；suno → `duration`，flowmusic → `length`；百炼不支持 |
| `--format` | | 音频格式（suno / OpenRouter / 百炼；flowmusic 不支持） |
| `--json` | | JSON 输入（文件、字符串，或 `-` 表示 stdin）；不带参数时逐字节原样发送，显式参数覆盖当前后端的对应键 |
| `--dry-run` | | 打印等价 curl，不调用 API |
| `--provider` | | 全局：引用命名 Provider（如 `openrouter`） |
| `--api-key` | | 全局：覆盖 API Key |
| `--api-base` | | 全局：覆盖 Base URL |
| `--output` | | 全局：下载目录（默认当前目录） |

---

## `--json`：后端原生 body + 显式参数覆盖

`--json` 写的是**当前后端的原生请求体**。不带任何参数时，原文**替换整个请求体**（逐字节原样发送）——不翻译、不补默认值，也不做校验。

与 CLI 参数同时出现时，只有**显式指定**的参数覆盖 body 中对应的键，其余键原样保留。四种后端的键名不同，覆盖写入的是**当前后端**使用的键：

| 后端 | 覆盖写入的键 | 无落点的参数 |
|---|---|---|
| APIMart suno | `prompt` / `style` / `title` / `duration` / `audio_format` | — |
| APIMart flowmusic | `sound_prompt` / `length` | `--format`（丢弃） |
| OpenRouter Lyria | 折进 `messages[0].content`，格式写入 `audio.format` | — |
| 百炼 fun-music | `input.prompt` / `input.lyrics` / `input.format` / `input.is_instrumental` | `--duration` / `--title`（丢弃） |

> 💡 suno 下带歌词时 `--prompt` 落到 `style`，否则落到 `prompt`。

要写**该后端的原生形状**时，直接照抄下列示例（不带参数即原样发送）：

```bash
# suno 专属字段（原生扁平形状）
aigc-cli music gen --provider apimart --json '{"model":"suno","prompt":"city pop","style_weight":0.6,"vocal_gender":"Female"}'

# flowmusic 专属字段
aigc-cli music gen --provider apimart --json '{"model":"flowmusic","sound_prompt":"rock","length":120,"bpm":"128","seed":"42"}'

# 百炼 Fun-Music：原生 input 嵌套
aigc-cli music gen --provider dashscope --json '{"model":"fun-music-v1","input":{"prompt":"城市民谣","gender":"male"}}'

# OpenRouter（chat/completions 形状）
aigc-cli music gen --provider openrouter --json '{"model":"google/lyria-3-pro-preview","messages":[{"role":"user","content":"ambient"}],"modalities":["text","audio"],"stream":true}'
```

只覆盖其中一个键、保留其余原生字段：

```bash
# suno 的 style_weight / vocal_gender 等私有键原样保留，只把 prompt 换成参数值
aigc-cli music gen --provider apimart --json '{"model":"suno","prompt":"lofi","style_weight":0.6,"vocal_gender":"Female"}' --prompt "city pop"
```

> 💡 body 必须是 JSON **对象**；`--json` 支持内联字符串、文件路径或 `-`（stdin），路径文件不存在时明确报 `file not found: <路径>`。行为类参数（`--dry-run`、`--provider`、`--api-key`、`--api-base`、`--output`）永远不写进 body。
> 💡 `--dry-run` / `--verbose` 打印真实端点与真实请求体，可直接验证。
> 💡 各后端端点不同：APIMart `{base}/music/generations`、OpenRouter `{base}/chat/completions`、百炼 DashScope 原生 `/api/v1/services/audio/music/generation`。

---

## 配置

`~/.config/aigc-cli/config.yaml`：

```yaml
providers:
  apimart:  { type: openai, api_key: "sk-xxx", base_url: "https://api.apimart.ai" }
  openrouter: { type: openai, api_key: "sk-or-xxx", base_url: "https://openrouter.ai/api/v1" }
  # 百炼：chat 走 compatible-mode；music 会自动改走原生 services 路径
  dashscope: { api_key: "sk-xxx", base_url: "https://{WorkspaceId}.cn-beijing.maas.aliyuncs.com/compatible-mode/v1" }
defaults:
  music:
    provider: apimart        # 改为 dashscope 即启用 Fun-Music
    model: suno              # dashscope 下应写 fun-music-v1 或 fun-music-preview
    # instrumental: false
    # duration: 120
    # format: mp3
```

---

## `--dry-run`

打印即将提交的等价 curl，不实际调用 API。

APIMart（suno 默认）：

```bash
aigc-cli music gen --prompt "city pop" --dry-run
```

```
curl -X POST https://api.apimart.ai/v1/music/generations \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d '{"custom":false,"instrumental":false,"model":"suno","prompt":"city pop","version":"v6"}'
```

APIMart（flowmusic）：

```
curl -X POST https://api.apimart.ai/v1/music/generations \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d '{"length":120,"model":"flowmusic","sound_prompt":"rock"}'
```

OpenRouter（Lyria，同步流式）：

```
curl -N -X POST https://openrouter.ai/api/v1/chat/completions \
  -H "Authorization: Bearer sk-or-xxx" \
  -H "Content-Type: application/json" \
  -d '{"model":"google/lyria-3-clip-preview","messages":[{"role":"user","content":"ambient"}],"modalities":["text","audio"],"audio":{"format":"mp3"},"stream":true}'
```

阿里云百炼（Fun-Music，同步；注意是原生 services 路径）：

```
curl -X POST 'https://{WorkspaceId}.cn-beijing.maas.aliyuncs.com/api/v1/services/audio/music/generation' \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d '{"input":{"prompt":"城市民谣"},"model":"fun-music-v1"}'
```

---

## MCP / Chat 代理

音乐生成能力已接入 MCP Server 与 Chat Agent，AI 代理可在对话中调用 `generate_music` 工具生成音乐。详见 [guide-mcp.md](guide-mcp.md) 与 [guide-chat.md](guide-chat.md)。

---

## 输出

生成的音频保存到输出目录（默认当前目录，可用 `--output` 指定），并打印 `Saved: <path>`：

- **APIMart**：每首曲目保存为 `music_<task_id>_<n>.<ext>`。
- **OpenRouter**：保存为 `audio_<unix 时间戳>.mp3`（`--format wav` 时为 `.wav`）。
- **阿里云百炼**：保存为 `music_<audio_id>_0.<ext>`。

---

## API 端点参考

| 端点 | 用途 | 适用 |
|---|---|---|
| `POST /v1/music/generations` | 提交音乐生成任务（异步） | APIMart ✅ |
| `GET /v1/music/tasks/{task_id}` | 查询音乐任务状态与结果 | APIMart ✅ |
| `POST /v1/chat/completions` | 同步流式音乐生成（`modalities: ["text","audio"]`） | OpenRouter ✅ |
| `POST /api/v1/services/audio/music/generation` | 同步音乐生成（DashScope 原生协议，返回音频 URL） | 阿里云百炼 ✅ |
