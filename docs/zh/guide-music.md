# 音乐生成

`aigc-cli music` 用自然语言提示词生成音乐。根据 Provider 自动选择两种后端：

- **APIMart（默认）**：异步任务模型 —— 提交任务 → 轮询 → 下载。`--model suno`（默认）或 `--model flowmusic`。
- **OpenRouter**：同步流式，`--model google/lyria-3-clip-preview`（默认，约 30 秒，mp3）或 `google/lyria-3-pro-preview`（数分钟，mp3/wav）。音频在一次调用中直接返回，**没有 task_id**。

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

# 查询任务：完成后自动下载音频
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

> `--model` 名称中包含 `flowmusic` 即走 flowmusic 后端，否则走 suno。

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

---

## 参数

| 参数 | 短参 | 说明 |
|---|---|---|
| `--prompt` | `-p` | 音乐描述 / 风格（suno 无歌词时作为 prompt，flowmusic 作为 sound_prompt） |
| `--model` | `-m` | 后端模型：`suno`（默认）/ `flowmusic`；OpenRouter 下为模型 ID |
| `--style` | | 风格（suno 的 style 字段；prompt 为空时兜底） |
| `--title` | | 曲目名称 |
| `--lyrics` | | 歌词（suno 非空时进入 custom 模式；flowmusic 作为 lyrics） |
| `--instrumental` | | 纯音乐（无人声） |
| `--duration` | `-d` | 时长（秒）；suno → `duration`，flowmusic → `length` |
| `--format` | | 音频格式（suno / OpenRouter；flowmusic 不支持） |
| `--json` | | JSON 输入（文件、字符串，或 `-` 表示 stdin） |
| `--dry-run` | | 打印等价 curl，不调用 API |
| `--provider` | | 全局：引用命名 Provider（如 `openrouter`） |
| `--api-key` | | 全局：覆盖 API Key |
| `--api-base` | | 全局：覆盖 Base URL |
| `--output` | | 全局：下载目录（默认当前目录） |

---

## `--json` 覆盖语义

Provider 专属字段通过 `--json` 传入。`--json` 的键**最后合并，始终覆盖** flag 与代码默认值（用于传递各后端独有的参数）。

```bash
# suno 专属字段：style_weight / vocal_gender / weirdness_constraint / version 等
aigc-cli music gen --prompt "city pop" --json '{"style_weight":0.6,"vocal_gender":"Female"}'

# flowmusic 专属字段：bpm / seed 等
aigc-cli music gen --prompt "rock" --model flowmusic --json '{"bpm":"128","seed":"42"}'

# OpenRouter
aigc-cli music gen --prompt "ambient" --provider openrouter --model google/lyria-3-pro-preview
```

---

## 配置

`~/.config/aigc-cli/config.yaml`：

```yaml
providers:
  apimart:  { type: openai, api_key: "sk-xxx", base_url: "https://api.apimart.ai" }
  openrouter: { type: openai, api_key: "sk-or-xxx", base_url: "https://openrouter.ai/api/v1" }
defaults:
  music:
    provider: apimart
    model: suno
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

---

## MCP / Chat 代理

音乐生成能力已接入 MCP Server 与 Chat Agent，AI 代理可在对话中调用 `generate_music` 工具生成音乐。详见 [guide-mcp.md](guide-mcp.md) 与 [guide-chat.md](guide-chat.md)。

---

## 输出

生成的音频保存到输出目录（默认当前目录，可用 `--output` 指定），并打印 `Saved: <path>`：

- **APIMart**：每首曲目保存为 `music_<task_id>_<n>.<ext>`。
- **OpenRouter**：保存为 `audio_<unix 时间戳>.mp3`（`--format wav` 时为 `.wav`）。

---

## API 端点参考

| 端点 | 用途 | 适用 |
|---|---|---|
| `POST /v1/music/generations` | 提交音乐生成任务（异步） | APIMart ✅ |
| `GET /v1/music/tasks/{task_id}` | 查询音乐任务状态与结果 | APIMart ✅ |
| `POST /v1/chat/completions` | 同步流式音乐生成（`modalities: ["text","audio"]`） | OpenRouter ✅ |
