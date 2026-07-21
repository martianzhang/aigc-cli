# 音频生成（文字转语音 TTS）

支持文字转语音（Text-to-Speech，TTS）和语音转文字（Speech-to-Text，STT），通过统一的 `audio` 命令提供。

> 📝 支持云端 API（OpenAI / OpenRouter / APIMart）和本地离线 TTS（sherpa-onnx）。

---

## 接口兼容性总览

目前已确认支持 TTS 的 Provider 如下，Yunwu（云雾 AI）暂未发现公开的 TTS/STT 端点：

| Provider | TTS 端点 | STT 端点 | 兼容性 |
|---|---|---|---|---|
| **OpenAI** | `POST /v1/audio/speech` | `POST /v1/audio/transcriptions` | 基准实现 |
| **OpenRouter** | `POST /api/v1/audio/speech` | `POST /api/v1/audio/transcriptions` | OpenAI 完全兼容，SDK 直连 |
| **APIMart** | `POST /v1/audio/speech` | `POST /v1/audio/transcriptions` | OpenAI 兼容 |
| **Yunwu** | ❌ 未发现 | ❌ 未发现 | — |
| **本地 TTS 服务** | `POST /v1/audio/speech` | — | 见下方"本地 TTS 方案" |

> Yunwu 官网自称"完全兼容 OpenAI API 协议"且聚合了 500+ 模型，理论上 `/v1/audio/speech` 透传可能也能通，但公开文档和定价页中均未列出 TTS 相关模型或接口，其视频 API 走的是自定义端点（`/v1/video/create` + `/v1/video/query`）。建议后续实现时实测确认。

检测逻辑：沿用现有 `base_url` 自动识别机制（OpenAI / OpenRouter / APIMart），无需新增 Provider 类型。Yunwu 走通用 OpenAI 兼容兜底逻辑。本地方案走 localhost 自动豁免，无需 API Key。

### 本地 TTS 方案

aigc-cli 的 `audio speak` 命令可直接对接以下本地 TTS 服务，无需修改代码：

| 方案 | 启动方式 | 端口 | 后端引擎 | 特点 |
|---|---|---|---|---|
| **openedai-speech** | `docker run ... ghcr.io/matatonic/openedai-speech` | 8000 | Piper（CPU）/ XTTS v2（GPU） | 轻量专用 TTS 服务，支持声音克隆 |
| **openai-edge-tts** | `pip install openai-edge-tts && openai-edge-tts` | 5050 | 微软 Edge TTS（免费在线） | 零 GPU，音质好，需联网 |
| **LocalAI** | `docker run ... localai/localai:latest` | 8080 | Piper/Coqui/VibeVoice/OmniVoice/Kokoro 等 | 全能，支持 STT + LLM + Image |

所有方案使用与 OpenAI 一致的请求体格式，aigc-cli 直连：

```bash
# openedai-speech（推荐）
aigc-cli audio speak --api-base "http://localhost:8000/v1" \
  --model "tts-1" --input "Hello" --voice "alloy"

# openai-edge-tts（零 GPU）
aigc-cli audio speak --api-base "http://localhost:5050/v1" \
  --model "tts-1" --input "你好" --voice "alloy" --format mp3

# LocalAI（全能）
aigc-cli audio speak --api-base "http://localhost:8080/v1" \
  --model "tts-1" --input "Hello" --voice "alloy"
```

### OpenAI TTS 参数

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `model` | string | 是 | `gpt-4o-mini-tts`（推荐）、`tts-1`、`tts-1-hd` |
| `input` | string | 是 | 要合成的文本 |
| `voice` | string | 是 | 13 种内置声音：`alloy`、`ash`、`ballad`、`coral`、`echo`、`fable`、`nova`、`onyx`、`sage`、`shimmer`、`verse`、`marin`、`cedar` |
| `response_format` | string | 否 | `mp3`（默认）、`opus`、`aac`、`flac`、`wav`、`pcm` |
| `speed` | number | 否 | 语速 0.25-4.0，默认 1.0 |
| `instructions` | string | 否 | 语气控制（如 "Speak in a cheerful tone"），仅 `gpt-4o-mini-tts` 支持 |

### OpenRouter TTS 参数

OpenRouter 透传 OpenAI 格式，额外支持 Provider 专属参数：

```json
{
  "model": "openai/gpt-4o-mini-tts-2025-12-15",
  "input": "Hello world",
  "voice": "alloy",
  "response_format": "mp3",
  "speed": 1.0,
  "provider": {
    "options": {
      "openai": { "instructions": "Speak in a warm tone." },
      "azure": { "style": "cheerful", "styledegree": 1.2 }
    }
  }
}
```

支持的模型（截至 2026 年 7 月）：

| 模型 Slug | Provider | 特点 |
|---|---|---|
| `openai/gpt-4o-mini-tts-2025-12-15` | OpenAI | `instructions` 语气控制，13 种声音 |
| `google/gemini-3.1-flash-tts-preview` | Google | 70+ 语言, 200+ 内联音频标签, 双说话人 |
| `mistralai/voxtral-mini-tts-2603` | Mistral | 零样本声音克隆, 多语言 |
| `microsoft/mai-voice-2` | Microsoft Azure | SSML 风格（cheerful/sad/excited） |
| `hexgrad/kokoro-82m` | 开源 | 轻量 82M 参数, 8 语言, 54 种预设声音 |
| `x-ai/grok-voice-tts-1.0` | xAI | 20+ 语言, 5 种声音, 内联语音标签 |
| `canopylabs/orpheus-3b-0.1-ft` | Canopy Labs | 英语, 7 种预设声音, 自然韵律 |
| `sesame/csm-1b` | Sesame | 对话式语音, 英语 |
| `zyphra/zonos-v0.1-*` | Zyphra | 英语, 英美口音 |

> 模型列表可通过 `GET /api/v1/models?output_modalities=speech` 动态发现。

### APIMart TTS 参数

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `model` | string | 是 | `gpt-4o-mini-tts`（目前唯一） |
| `input` | string | 是 | 最大 4096 字符 |
| `voice` | string | 是 | 6 种：`alloy`、`echo`、`fable`、`onyx`、`nova`、`shimmer` |
| `response_format` | string | 否 | `wav`（默认）、`opus`、`aac`、`flac`、`pcm` |
| `speed` | number | 否 | 0.25-4.0，默认 1.0 |

---

## 响应格式

TTS 端点返回**二进制音频流**（非 JSON），需根据 `response_format` 保存为对应扩展名：

| response_format | Content-Type | 扩展名 | 说明 |
|---|---|---|---|
| `mp3` | `audio/mpeg` | `.mp3` | 通用压缩格式 |
| `opus` | `audio/opus` | `.opus` | 低延迟流式 |
| `aac` | `audio/aac` | `.aac` | 数字音频压缩 |
| `flac` | `audio/flac` | `.flac` | 无损压缩 |
| `wav` | `audio/wav` | `.wav` | 未压缩，低延迟 |
| `pcm` | `audio/pcm` | `.pcm` | 纯裸采样（24kHz 16-bit signed LE） |

OpenRouter 仅支持 `mp3` 和 `pcm`。

---

## 语音转文字（STT）

所有 Provider 也支持 STT，通过 `POST /v1/audio/transcriptions` 端点：

### OpenAI 格式（JSON body + base64）

```bash
AUDIO_BASE64=$(base64 < audio.wav | tr -d '\n')

curl https://openrouter.ai/api/v1/audio/transcriptions \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "openai/whisper-1",
    "input_audio": {
      "data": "'"$AUDIO_BASE64"'",
      "format": "wav"
    }
  }'
```

### Multipart/form-data（兼容 OpenAI SDK）

```bash
curl https://openrouter.ai/api/v1/audio/transcriptions \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -F file="@audio.wav" \
  -F model="openai/whisper-1"
```

### 响应格式

```json
{
  "text": "Hello, this is a test.",
  "usage": {
    "seconds": 9.2,
    "total_tokens": 113,
    "input_tokens": 83,
    "output_tokens": 30,
    "cost": 0.000508
  }
}
```

支持的音频格式：`wav`、`mp3`、`flac`、`m4a`、`ogg`、`webm`、`aac`。

---

## Provider 差异对比

| 维度 | OpenAI | OpenRouter | APIMart |
|---|---|---|---|
| **TTS 端点** | `/v1/audio/speech` | `/api/v1/audio/speech` | `/v1/audio/speech` |
| **可用模型数** | 3 | 10+（聚合多厂商） | 1 |
| **声音数** | 13 内置 + 自定义 | 随模型变化 | 6 |
| **输出格式** | mp3/opus/aac/flac/wav/pcm | mp3/pcm | wav/opus/aac/flac/pcm |
| **`instructions`** | ✅ | 透传（OpenAI 模型） | ❌ |
| **`speed`** | ✅ 0.25-4.0 | 透传 | ✅ 0.25-4.0 |
| **流式响应** | ✅ chunk transfer | 二进制流 | 二进制流 |
| **STT 支持** | ✅ | ✅ | ✅ |
| **定价** | 按字符 | 按字符（模型各异） | 按字符 |
| **自定义声音** | ✅ 需申请 | ❌ | ❌ |
| **最大输入** | 无明确限制 | 无明确限制 | 4096 字符 |

---

## 与现有架构的对比（视频 `generate-audio`）

现有 `video` 命令已支持 `--generate-audio` 参数，用于给生成的视频配上 AI 音频。这是通过 OpenRouter 视频 API 的 `generate_audio` 字段实现的**视频配乐**功能，与独立的 TTS/STT 端点不同：

| 维度 | 现有 `video --generate-audio` | 新增 `audio` 命令 |
|---|---|---|
| 端点 | `POST /v1/videos/generations` | `POST /v1/audio/speech` |
| 用途 | 给生成的视频附加音频 | 独立的文字转语音 |
| 输出 | 带音频的视频文件 | 纯音频文件（mp3/wav 等） |
| 定制能力 | 无（由模型决定） | 可选择声音、语速、格式 |

两者是互补关系，不冲突。

---

## 实现思路

### 命令行设计

```bash
# TTS：文字转语音
aigc-cli audio speak --model "openai/gpt-4o-mini-tts" \
  --input "你好，世界" \
  --voice "alloy" \
  --format "mp3"

# 从文件读取
aigc-cli audio speak --model "openai/gpt-4o-mini-tts" --input text.txt --voice nova

# 从 stdin 读取
echo "Hello world" | aigc-cli audio speak --model "openai/gpt-4o-mini-tts" --voice alloy

# 生成后自动播放
aigc-cli audio speak --model "openai/gpt-4o-mini-tts" --input "Hi" --voice alloy --play

# STT：语音转文字（自动 multipart upload）
aigc-cli audio transcribe --model "openai/whisper-1" --input recording.wav

# STT：指定语言
aigc-cli audio transcribe --model "openai/whisper-1" --input speech.mp3 --language en

# STT：base64 输入
cat recording.wav | base64 | aigc-cli audio transcribe --model "openai/whisper-1" --format wav

# Dry-run 查看等价 curl
aigc-cli audio speak --model "openai/gpt-4o-mini-tts" --input "Hello" --voice alloy --dry-run
```

### 文件拆分（参考 image/video 模式）

| 文件 | 职责 | 预估行数 |
|---|---|---|
| `audio.go` | 命令定义、`runAudioSpeak`、`runAudioTranscribe`、标志注册、`init` | ~280 行 |
| `audio_request.go` | 请求构建（`buildAudioSpeechRequest`）、dry-run curl 生成、maskKey | ~80 行 |
| `audio_helpers.go` | 音频文件保存、格式扩展名映射、Content-Type 解析 | ~70 行 |

### Provider 自动适配

已有 `internal/provider/detect.go` 根据 `base_url` 自动识别，直接复用：

```go
switch provider.Detect(baseURL) {
case provider.OpenRouter:
    // POST /api/v1/audio/speech
case provider.APIMart:
    // POST /v1/audio/speech
default: // OpenAI / 通用中转
    // POST /v1/audio/speech
}
```

三个 Provider 的端点格式完全一致（OpenAI 兼容），差异仅在 URL 路径前缀。

### 请求参数

```go
type AudioSpeechRequest struct {
    Model          string  `json:"model"`
    Input          string  `json:"input"`
    Voice          string  `json:"voice"`
    ResponseFormat string  `json:"response_format,omitempty"`
    Speed          float64 `json:"speed,omitempty"`
    Instructions   string  `json:"instructions,omitempty"` // OpenAI gpt-4o-mini-tts only
}
```

### 关键设计决策

1. **二进制流处理**：TTS 返回裸音频流，响应体直接写文件。需要与 JSON 响应的 API（如 STT）区分处理
2. **输出文件命名**：自动以 `audio_<timestamp>.<ext>` 命名，或通过 `--output` 指定
3. **格式默认值**：
   - TTS 默认 `mp3`（各 Provider 都支持）
   - STT 默认 `json`（返回转录文本）
4. **STT 文件上传**：需要支持 base64 JSON 和 multipart/form-data 两种方式
5. **流式播放**：可选的 `--play` 参数，生成后自动调用系统播放器（参考 `--preview`）

### MCP 工具

参考 `tools_generate.go` 的模式，新增 MCP 工具：

```go
mcp.WithTool("generate_speech",
    mcp.Description("Convert text to speech audio"),
    mcp.WithString("model", mcp.Description("TTS model")),
    mcp.WithString("input", mcp.Description("Text to speak"), mcp.Required()),
    mcp.WithString("voice", mcp.Description("Voice name"), mcp.Required()),
    mcp.WithString("format", mcp.Description("Audio format: mp3, wav, opus")),
)
```

---

## 本地 TTS（离线语音合成）

`aigc-cli` 支持通过 sherpa-onnx 在本地运行 TTS，无需联网、无需 API Key，数据不出设备。

### 快速开始

```bash
# 1. 下载默认模型（kokoro，支持中英日韩法）
aigc-cli audio init --model kokoro

# 2. 语音合成（默认中文女声晓晓）
aigc-cli audio speak --local --input "你好，欢迎使用本地语音合成系统。"

# 3. 或使用 tts 别名
aigc-cli audio tts --local --input "Hello, this is local TTS."
```

### 可用 TTS 模型

| 模型 ID | 语言 | 架构 | 说明 |
|---|---|---|---|
| `kokoro` | 🌐 中/英/日/韩/法 | Kokoro | **默认**，82M 参数，53 种音色，中英混合最佳 |
| `kokoro-en` | 🇺🇸 英语 | Kokoro | 纯英文版，体积更小 |
| `vits-zh-ll` | 🇨🇳 中文 | VITS | 5 说话人 |
| `vits-zh-hf-eula` | 🇨🇳 中文 | VITS | 804 说话人，语料丰富 |
| `vits-zh-aishell3` | 🇨🇳 中文 | VITS | 标准女声 |
| `vits-cantonese` | 🇭🇰 粤语 | VITS | 粤语 TTS |
| `matcha-zh-en` | 🇨🇳🇺🇸 中英双语 | Matcha-TTS | 需额外下载 vocoder |
| `matcha-en` | 🇺🇸 英语 | Matcha-TTS | 高质量英文 |
| `vits-ljs` | 🇺🇸 美式英语 | VITS | LJSpeech 女声 |
| `vits-vctk` | 🇬🇧 英式英语 | VITS | 109 说话人 |

### 下载模型

```bash
# 列出所有可用模型
aigc-cli audio init --list

# 按类型筛选
aigc-cli audio init --list --type tts

# 下载指定模型
aigc-cli audio init --model kokoro
aigc-cli audio init --model vits-zh-ll --model vits-ljs

# 从任意 URL 下载（不受注册表限制）
aigc-cli audio init --url https://example.com/model.tar.bz2 --name my-model

# 查看已安装的模型
aigc-cli audio init --list-installed
```

### 语音合成

```bash
# 使用默认模型（kokoro，中文女声晓晓）
aigc-cli audio tts --local --input "你好，世界"

# 指定模型
aigc-cli audio tts --local --model vits-ljs --input "Hello world"

# 指定音色
aigc-cli audio tts --local --voice zf_xiaoxiao --input "你好"
aigc-cli audio tts --local --voice zm_yunjian --input "你好，我是男声"

# 或直接使用编号
aigc-cli audio tts --local --voice 45 --input "你好"

# 生成后自动播放
aigc-cli audio tts --local --input "你好" --play
```

### Kokoro 音色列表

`kokoro` 模型有 53 种音色（SID 0-52）：

```bash
# 查看所有音色
aigc-cli audio init --list-voices --model kokoro
```

| 音色名 | ID | 语言/性别 |
|---|---|---|
| `af_alloy` .. `af_sky` | 0-10 | 🇺🇸 美式英语女声（11 种） |
| `am_adam` .. `am_santa` | 11-19 | 🇺🇸 美式英语男声（9 种） |
| `bf_alice` .. `bf_lily` | 20-23 | 🇬🇧 英式英语女声（4 种） |
| `bm_daniel` .. `bm_lewis` | 24-27 | 🇬🇧 英式英语男声（4 种） |
| `ef_dora` | 28 | 🇪🇸 西班牙语女声 |
| `em_alex` | 29 | 🇪🇸 西班牙语男声 |
| `ff_siwis` | 30 | 🇫🇷 法语女声 |
| `hf_alpha` / `hf_beta` | 31-32 | 🇮🇳 印地语女声 |
| `hm_omega` / `hm_psi` | 33-34 | 🇮🇳 印地语男声 |
| `if_sara` | 35 | 🇮🇹 意大利语女声 |
| `im_nicola` | 36 | 🇮🇹 意大利语男声 |
| `jf_alpha` .. `jf_tebukuro` | 37-40 | 🇯🇵 日语女声（4 种） |
| `jm_kumo` | 41 | 🇯🇵 日语男声 |
| `pf_dora` | 42 | 🇧🇷 葡萄牙语女声 |
| `pm_alex` / `pm_santa` | 43-44 | 🇧🇷 葡萄牙语男声 |
| **`zf_xiaobei`** | **45** | **🇨🇳 中文女声** |
| **`zf_xiaoni`** | **46** | **🇨🇳 中文女声** |
| **`zf_xiaoxiao`** | **47** | **🇨🇳 中文女声（默认）** |
| **`zf_xiaoyi`** | **48** | **🇨🇳 中文女声** |
| **`zm_yunjian`** | **49** | **🇨🇳 中文男声** |
| **`zm_yunxi`** | **50** | **🇨🇳 中文男声** |
| **`zm_yunxia`** | **51** | **🇨🇳 中文男声** |
| **`zm_yunyang`** | **52** | **🇨🇳 中文男声** |

### 配置

```yaml
# ~/.config/aigc-cli/config.yaml
defaults:
  audio:
    local: true                    # 优先使用本地模型
    speak_model: kokoro            # 本地 TTS 模型 ID
    voice: "zf_xiaoxiao"           # 默认音色（支持名字或数字）
    format: wav                    # 输出格式
```

启用后可直接运行：

```bash
aigc-cli audio speak --input "你好"  # 自动使用本地模型，无需 --local
```

### 子命令别名

```bash
aigc-cli audio speak --input "你好"   # 完整命令
aigc-cli audio tts --input "你好"     # 等价的别名
```

### 技术说明

- **引擎**：sherpa-onnx（C++，通过 Go binding 调用）
- **编译**：需要 CGO（macOS/Linux 默认开启，无需额外配置）
- **预编译库**：自动下载，用户无需安装 C 工具链
- **模型文件**：通过 `audio init` 从 HuggingFace / GitHub Releases 下载
- **输出格式**：仅支持 WAV（本地模式）
- **默认音色**：zf_xiaoxiao（晓晓，中文女声，SID 47）

---

## 参考文档

| Provider | 参考来源 |
|---|---|
| OpenAI TTS | https://developers.openai.com/api/docs/guides/text-to-speech |
| OpenRouter TTS | https://openrouter.ai/docs/guides/overview/multimodal/tts |
| OpenRouter STT | https://openrouter.ai/docs/guides/overview/multimodal/stt |
| OpenRouter Blog (Audio API 发布) | https://openrouter.ai/blog/announcements/announcing-audio-apis/ |
| OpenRouter TTS 模型列表 | https://openrouter.ai/collections/text-to-speech-models |
| APIMart TTS | https://docs.apimart.ai/en/api-reference/audios/tts |

---

## 实现状态

| 模块 | 状态 | 说明 |
|---|---|---|
| `internal/types/audio_types.go` | ✅ 已完成 | AudioSpeechRequest、AudioTranscribeRequest/Response 类型 |
| `internal/client/client_audio.go` | ✅ 已完成 | AudioSpeech（二进制）、AudioTranscribe（JSON）、AudioTranscribeMultipart |
| `internal/client/client.go` | ✅ 已完成 | 路径和超时常量 |
| `internal/client/interface.go` | ✅ 已完成 | APIClient 接口新增三个方法 |
| `cmd/audio.go` | ✅ 已完成 | `audio speak` + `audio transcribe` 子命令 |
| `cmd/audio_request.go` | ✅ 已完成 | 请求构建 + dry-run curl |
| `cmd/audio_helpers.go` | ✅ 已完成 | 文件保存 + 格式解析 |
| `internal/types/types_config.go` | ✅ 已完成 | AudioDefaults 配置结构体 |
| `internal/mcp/tools_generate.go` | ✅ 已完成 | generate_speech、transcribe_audio 处理器 |
| `internal/mcp/server.go` | ✅ 已完成 | MCP 工具注册 + description builder |
| `docs/guide-audio.md` | ✅ 已完成 | 本文档 |
