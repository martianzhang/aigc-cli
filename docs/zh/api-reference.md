# API 接口参考来源

> 本文档记录本项目中各 API 中转站的接口规范来源，方便后续修改代码时快速查找参考文档。

---

## 概览

| Provider | 官方文档 | 实现位置 |
|---|---|---|
| **APIMart**（apimart.ai） | [docs.apimart.ai](https://docs.apimart.ai/en) | `internal/client/client.go` |
| **OpenAI** | [platform.openai.com/docs/api-reference](https://platform.openai.com/docs/api-reference) | `internal/client/client.go` |
| **OpenRouter** | [openrouter.ai/docs](https://openrouter.ai/docs) | `internal/client/openrouter.go` |
| **OpenLux**（api.openlux.ai） | [doc.openlux.ai](https://doc.openlux.ai/en/tutorials/00-intro) | `internal/client/client.go` |
| 其他 OpenAI 兼容中转 | 各自服务商的文档 | `internal/client/client.go` |

---

## 1. APIMart（apimart.ai）

**官方文档**：[https://docs.apimart.ai/en](https://docs.apimart.ai/en)

APIMart 是一个 AI API 中转服务平台，兼容 OpenAI 格式并扩展了异步任务机制。
本项目中的 APIMart 接口参考自其公开文档中的以下章节：

| 端点 | 用途 | 参考章节 |
|---|---|---|
| `POST /v1/images/generations` | 文生图（异步 task + 同步 OpenAI 兼容） | [Images](https://docs.apimart.ai/en) |
| `POST /v1/videos/generations` | 文生视频（异步 task） | [Videos](https://docs.apimart.ai/en) |
| `POST /v1/videos/{task_id}/remix` | VEO3 视频续拍 | [Videos](https://docs.apimart.ai/en) |
| `POST /v1/audio/speech` | 文字转语音（TTS） | [TTS](https://docs.apimart.ai/en/api-reference/audios/tts) |
| `POST /v1/audio/transcriptions` | 语音转文字（STT） | [Whisper](https://docs.apimart.ai/en/api-reference/audios/whisper-1) |
| `POST /v1/midjourney/generations/{action}` | Midjourney 生成（imagine/blend/upscale 等） | [Midjourney](https://docs.apimart.ai/en) |
| `GET /v1/midjourney/{task_id}` | 查询 MJ 任务状态 | [Midjourney](https://docs.apimart.ai/en) |
| `POST /v1/music/generations` | 音乐生成（异步 task，`model` 选 suno / flowmusic） | [Suno](https://docs.apimart.ai/en/api-reference/audios/suno/generation) / [Flow Music](https://docs.apimart.ai/en/api-reference/audios/flow-music/music) |
| `GET /v1/music/tasks/{task_id}` | 查询音乐任务状态 | [Suno Overview](https://docs.apimart.ai/en/api-reference/audios/suno/overview) |
| `POST /v1/uploads/images` | 上传图片 | [Uploads](https://docs.apimart.ai/en) |
| `GET /v1/tasks/{task_id}` | 查询异步任务状态 | [Tasks](https://docs.apimart.ai/en) |
| `GET /v1/balance` | Token 余额查询 | [Balance](https://docs.apimart.ai/en) |
| `GET /v1/user/balance` | 用户余额查询 | [Balance](https://docs.apimart.ai/en) |
| `GET /api/marketplace/models` | 模型市场列表（免认证） | [Marketplace](https://docs.apimart.ai/en) |
| `GET /api/pricing/model` | 模型定价详情（免认证） | [Pricing](https://docs.apimart.ai/en) |

> 注意：APIMart 的 `/v1/images/generations` 支持异步 task 返回值格式，与标准 OpenAI 同步格式不同。
> 代码中通过 `provider.IsAPIMart()` 检测到 APIMart 域名时自动走异步路径。

---

## 2. OpenAI

**官方文档**：[https://platform.openai.com/docs/api-reference](https://platform.openai.com/docs/api-reference)

本项目实现了 OpenAI 的标准 API 接口，参考以下官方章节：

| 端点 | 用途 | 参考链接 |
|---|---|---|
| `POST /v1/images/generations` | 文生图（同步） | [Create image](https://platform.openai.com/docs/api-reference/images/create) |
| `POST /v1/chat/completions` | AI 对话（流式 + 非流式） | [Create chat completion](https://platform.openai.com/docs/api-reference/chat/create) |
| `POST /v1/audio/speech` | 文字转语音（TTS） | [Create speech](https://platform.openai.com/docs/api-reference/audio/createSpeech) |
| `POST /v1/audio/transcriptions` | 语音转文字（STT） | [Create transcription](https://platform.openai.com/docs/api-reference/audio/createTranscription) |
| `GET /v1/models` | 模型列表 | [List models](https://platform.openai.com/docs/api-reference/models/list) |
| `GET /v1/models/{model}` | 单个模型详情 | [Retrieve model](https://platform.openai.com/docs/api-reference/models/retrieve) |

> OpenAI 标准是所有兼容 API 的基线。本项目中的 OpenAI 实现也是其他中转服务（APIMart 同步模式、第三方中转）的后备默认路径。

### OpenAI Responses API（chat 专用）

Provider 配置 `type: openai_responses` 时，`chat` 命令改走 OpenAI Responses 协议：

| 端点 | 用途 |
|---|---|
| `POST {base_url}/responses` | AI 对话（支持 reasoning + function tools） |

用于部分推理模型（如 GPT-6 系列）在 `/v1/chat/completions` 上因 `reasoning_effort` 与函数工具冲突而返回 HTTP 400 的场景。仅影响 `chat` 命令。

---

## 3. OpenRouter（openrouter.ai）

**官方文档**：[https://openrouter.ai/docs](https://openrouter.ai/docs)

OpenRouter 提供统一的 API 网关，支持多种模型的图片和视频生成。
本项目实现中 OpenRouter 的特殊接口参考以下文档：

### 3.1 图片生成

OpenRouter 有两种图片生成路径：

| 端点 | 用途 | 参考链接 |
|---|---|---|
| `POST /v1/images` | **专用 Image API** — 用于 DALL-E、GPT-Image 等专用图片模型 | [Image Generation Guide](https://openrouter.ai/docs/guides/overview/multimodal/image-generation) |
| `POST /v1/responses` | **Responses API** — 用于 Gemini、Claude 等支持图片输出的多模态模型 | [Responses API](https://openrouter.ai/docs/guides/overview/multimodal/image-generation) |

路由策略（`cmd/image.go` 中的 `imageStrategies`）：
1. 非 DALL-E/GPT-Image 模型 → Responses API（`POST /v1/responses`）
2. DALL-E/GPT-Image 模型 → 专用 Image API（`POST /v1/images`）

### 3.2 视频生成

| 端点 | 用途 | 参考链接 |
|---|---|---|
| `POST /v1/videos` | 提交视频生成任务（异步） | [Video Generation Guide](https://openrouter.ai/docs/guides/overview/multimodal/video-generation) |
| `GET /v1/videos/{job_id}` | 查询视频任务状态 | [Poll video status](https://openrouter.ai/docs/api/api-reference/video-generation/get-videos) |
| `GET {polling_url}` | 轮询视频任务进度 | [Video Generation Guide](https://openrouter.ai/docs/guides/overview/multimodal/video-generation) |

### 3.3 模型发现（免认证）

| 端点 | 用途 | 参考链接 |
|---|---|---|
| `GET /v1/images/models` | 图片模型列表（含参数/能力描述） | [List image models](https://openrouter.ai/docs/api/api-reference/images/list-image-models) |
| `GET /v1/videos/models` | 视频模型列表 | [List video models](https://openrouter.ai/docs/api/api-reference/video-generation/list-videos-models) |
| `GET /v1/models` | 通用模型列表 | [List models](https://openrouter.ai/docs/api/api-reference/models/get-models) |

### 3.5 音频

| 端点 | 用途 | 参考链接 |
|---|---|---|
| `POST /api/v1/audio/speech` | 文字转语音（TTS）— OpenAI 兼容 | [TTS Guide](https://openrouter.ai/docs/guides/overview/multimodal/tts) |
| `POST /api/v1/audio/transcriptions` | 语音转文字（STT）— OpenAI 兼容 | [STT Guide](https://openrouter.ai/docs/guides/overview/multimodal/stt) |
| `GET /v1/models?output_modalities=speech` | 发现 TTS 模型 | [Model Discovery](https://openrouter.ai/docs/guides/overview/multimodal/tts#model-discovery) |
| `GET /v1/models?output_modalities=transcription` | 发现 STT 模型 | [STT Model Discovery](https://openrouter.ai/docs/guides/overview/multimodal/stt#model-discovery) |

### 3.6 音乐生成（Lyria-3）

音乐模型走标准 chat completions 端点，通过 `modalities` 开启音频输出，且**必须流式**（`stream: true`）；音频以 base64 分片返回在 `choices[].delta.audio.data`。

| 端点 | 用途 | 参考链接 |
|---|---|---|
| `POST /v1/chat/completions` | Google Lyria-3 音乐生成（`modalities: ["text","audio"]`，`stream: true`） | [Lyria 3 Clip](https://openrouter.ai/google/lyria-3-clip-preview) / [Lyria 3 Pro](https://openrouter.ai/google/lyria-3-pro-preview) |

> 模型：`google/lyria-3-clip-preview`（约 30 秒 mp3）、`google/lyria-3-pro-preview`（数分钟 mp3/wav）。该模型可能受地区限制（403），必要时经代理访问。

### 3.4 OpenRouter 特有 Header

| Header | 用途 | 参考 |
|---|---|---|
| `HTTP-Referer` | 标识请求来源（`OPENAI_REFERER` 环境变量） | [OpenRouter docs](https://openrouter.ai/docs) |
| `X-OpenRouter-Title` | 应用名称（`OPENAI_APP_TITLE` 环境变量） | [OpenRouter docs](https://openrouter.ai/docs) |

---

## 4. OpenLux（api.openlux.ai）

**官方文档**：[https://doc.openlux.ai/en/tutorials/00-intro](https://doc.openlux.ai/en/tutorials/00-intro)

| 端点 | 用途 | 参考 |
|---|---|---|
| `POST /v1/video/create` | 提交视频生成任务 | OpenLux 官方文档 |
| `GET /v1/video/query?id={id}` | 查询视频任务状态 | OpenLux 官方文档 |

> OpenLux 由 `internal/provider/detect.go` 中的 `openluxDomains` 列表检测。

---

## 5. 通用 OpenAI 兼容模式

当 `base_url` 不属于上述任何已知 Provider 时，默认走 **OpenAI 兼容同步模式**：

| 端点 | 用途 |
|---|---|
| `POST /v1/images/generations` | 文生图（同步） |
| `POST /v1/chat/completions` | AI 对话 |
| `POST /v1/audio/speech` | 文字转语音（TTS） |
| `POST /v1/audio/transcriptions` | 语音转文字（STT） |
| `GET /v1/models` | 模型列表 |
| `GET /v1/models/{model}` | 单个模型详情 |

适用于任意第三方 OpenAI 兼容中转服务（如 `api.302.ai`、`api.gptgod.ai` 等）。

---

## 6. Provider 检测机制

`internal/provider/detect.go` 负责根据 `base_url` 自动识别 Provider：

```go
APIMart:    域名包含 apimart.ai / apib.ai / aiuxu.com / aishuch.com
OpenRouter: 域名包含 openrouter.ai
OpenLux:    域名包含 openlux.ai
Zeekai:     域名包含 zeekai.cc（图生图自动改走 POST /images/edits）
默认:       OpenAI 兼容（任何未匹配的 URL）
```

新增 Provider 时只需：
1. 在 `internal/provider/detect.go` 的 `Type` 枚举中添加新类型
2. 添加对应的域名列表
3. 在 `internal/client/` 中实现对应的客户端方法
4. 在 `cmd/` 的策略表（`imageStrategies` / `videoStrategies`）中添加路由规则

---

## 7. 各命令策略路由

以下命令使用 match-run 策略表进行 Provider 分发：

### `image` 策略表（`cmd/image.go`）

| 优先级 | 匹配条件 | 路由目标 | 使用的端点 |
|---|---|---|---|
| 1 | OpenRouter + 非 DALL-E/GPT-Image 模型 | Responses API | `POST /v1/responses` |
| 2 | OpenRouter + 其他 | 专用 Image API | `POST /v1/images` |
| 3 | APIMart 域名 | 异步任务 | `POST /v1/images/generations` (async) |
| 4 | 默认（兜底） | OpenAI 兼容同步 | `POST /v1/images/generations` (sync) |

### `video` 策略表（`cmd/video.go`）

| 优先级 | 匹配条件 | 路由目标 | 使用的端点 |
|---|---|---|---|
| 1 | OpenRouter | OpenRouter 视频 API | `POST /v1/videos` |
| 2 | OpenLux | OpenLux 统一视频 API | `POST /v1/video/create` |
| 3 | 默认（兜底） | APIMart 异步任务 | `POST /v1/videos/generations` |

### `music` 策略表（`cmd/music_generate.go` / `cmd/music_shared.go`）

按 Provider 类型分派（非 match-run 表）：

| 条件 | 路由目标 | 使用的端点 |
|---|---|---|
| OpenRouter 域名 | 同步流式（Google Lyria-3），一次调用返回音频 | `POST /v1/chat/completions`（`modalities: ["text","audio"]`, `stream: true`） |
| 默认（兜底，含 APIMart） | APIMart 异步任务 → 轮询 → 下载 | `POST /v1/music/generations` → `GET /v1/music/tasks/{task_id}` |

`suno` / `flowmusic` 由请求体的 `model` 字段区分（名称含 `flowmusic` 走 flowmusic）。

### `models` 策略表（`cmd/models.go`）

| 条件 | 路由目标 | 端点 |
|---|---|---|
| OpenRouter + `--type image/video` | OpenRouter 模型发现 | `GET /v1/images\|videos/models` |
| 其他 + `--type` / `--price` | APIMart 市场 | `GET /api/marketplace/models` |
| 默认（无参数） | OpenAI 兼容 | `GET /v1/models` |

---

## 更新记录

| 日期 | 变更 | 说明 |
|---|---|---|
| 2026-06-29 | 初始创建 | 记录各 Provider 的 API 参考来源 |
| 2026-09-12 | 新增 music | APIMart suno/flowmusic 异步、OpenRouter Lyria-3 同步流式 |
