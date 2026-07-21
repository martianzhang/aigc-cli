# 安装与配置

## 安装

### go install

```bash
go install github.com/martianzhang/aigc-cli@latest
```

### 从源码构建

```bash
git clone https://github.com/martianzhang/aigc-cli.git
cd aigc-cli
make build
```

### Makefile 常用命令

```bash
make          # 编译
make run ARGS="image --help"   # 运行查看帮助
make clean    # 清理产物
make lint     # 静态检查
make test     # 运行测试
make cover    # 测试覆盖率
make release  # 交叉编译（所有平台）
```

## 本地生成

aigc-cli 可以连接本地运行的 OpenAI 兼容服务进行图片和音频生成，无需 API Key。aigc-cli 会自动检测 localhost 地址并跳过 Authorization 头。

> 所有本地方案的 API 请求格式与 OpenAI 兼容，aigc-cli 无需修改代码即可对接。

### 图片生成

```bash
# Ollama（默认端口 11434）
export OPENAI_BASE_URL="http://localhost:11434"
aigc-cli image --model "x/z-image-turbo" --prompt "a cat"
```

需要先 pull 图片生成模型：

```bash
ollama pull x/z-image-turbo      # 推荐，6B 参数，Apache 2.0
ollama pull x/flux2-klein:4b     # 4B，商业友好
ollama pull x/flux2-klein:9b     # 9B，仅非商业用途（FLUX 许可）
```

> ⚠️ Ollama 仅支持 `response_format=b64_json`，不支持 URL 格式，aigc-cli 会自动处理 base64 解码。

### 音频生成（TTS）

aigc-cli 的 `audio speak` 命令可直接对接以下本地 TTS 服务：

<details>
<summary><b>openedai-speech</b> — 轻量专用 TTS 服务器（推荐）</summary>

```bash
# Docker 启动
docker run -d -p 8000:8000 ghcr.io/matatonic/openedai-speech

# aigc-cli 直连
aigc-cli audio speak \
  --api-base "http://localhost:8000/v1" \
  --model "tts-1" \
  --input "Hello world" \
  --voice "alloy"

# 也支持 Piper TTS（CPU）和 XTTS v2（GPU 声音克隆）
```
</details>

<details>
<summary><b>openai-edge-tts</b> — 零 GPU，基于微软 Edge TTS（免费）</summary>

```bash
# 安装
pip install openai-edge-tts

# 启动（默认端口 5050）
openai-edge-tts

# aigc-cli 直连
aigc-cli audio speak \
  --api-base "http://localhost:5050/v1" \
  --model "tts-1" \
  --input "你好世界" \
  --voice "alloy"

# 支持 mp3/opus/aac/flac/wav/pcm 格式，speed 0.25-4.0
# 需网络连接（调用微软 Edge TTS 服务），免费且音质好
```
</details>

<details>
<summary><b>LocalAI</b> — 全能 AI 服务（LLM + TTS + STT + Image）</summary>

```bash
# Docker 启动（端口 8080）
docker run -p 8080:8080 localai/localai:latest

# TTS 示例
aigc-cli audio speak \
  --api-base "http://localhost:8080/v1" \
  --model "tts-1" \
  --input "Hello" \
  --voice "alloy"

# STT 示例
aigc-cli audio transcribe \
  --api-base "http://localhost:8080/v1" \
  --model "whisper-1" \
  --input speech.wav

# 后端引擎：Piper / Coqui / VibeVoice / OmniVoice / Kokoro / Qwen-TTS
# 也支持图片生成、视频理解、对话等
```
</details>

### 本地方案对比

| 能力 | Ollama | openedai-speech | openai-edge-tts | LocalAI |
|---|---|---|---|---|
| **图片生成** | ✅ (`x/z-image-turbo` 等) | ❌ | ❌ | ✅ (多后端) |
| **音频 TTS** | ❌ | ✅ (Piper/XTTS) | ✅ (Edge TTS) | ✅ (5+ 后端) |
| **音频 STT** | ❌ | ❌ | ❌ | ✅ (whisper) |
| **视频生成** | ❌ | ❌ | ❌ | ❌ |
| **对话/LLM** | ✅ | ❌ | ❌ | ✅ |
| **GPU 需要** | 可选 | 可选 | 不需要 | 可选 |
| **启动复杂度** | 极简 | 简单 | 简单 | 中等 |
| **许可证** | MIT | Apache 2.0 | MIT | MIT |

## 配置 API Key

三种设置方式（优先级从高到低）：

```bash
# 方式一：命令行参数
aigc-cli image --prompt "..." --api-key "sk-xxx"

# 方式二：环境变量
export OPENAI_API_KEY="sk-xxx"

# 方式三：配置文件
```

## 配置文件

默认位置 `~/.config/aigc-cli/config.yaml`：

```yaml
api_key: "sk-xxx"

# API 地址（默认 https://api.apimart.ai）
# base_url: "https://api.apimart.ai"

# 生成模式：auto（自动检测）、sync（同步）、async（异步任务）
# 默认 auto，会根据 base_url 自动识别
# mode: "auto"

# HTTP 代理
# 也可通过 HTTP_PROXY 环境变量设置
http_proxy: "http://127.0.0.1:7890"

# 图片下载目录（默认当前目录 "."）
# output_dir: "./downloads"

# 全局超时（秒，默认 180），各模态 defaults.*.timeout 会覆盖此值
# timeout: 300

defaults:
  image:
    model: "gpt-image-2-official"
    size: "3:1"
    resolution: "1k"
    quality: "low"
    output_format: "png"
    # timeout: 300                 # HTTP 超时秒数（覆盖 timeout）

  video:
    model: "doubao-seedance-2.0"
    # timeout: 600                 # HTTP 超时秒数（视频生成更慢）
```

完整示例见 [config.example.yaml](config.example.yaml)。

### 提示词文件

加 `--save-prompt` 可将提示词保存到 `image_{task_id}.md` 文件，方便追溯：

```bash
aigc-cli image --prompt "A red fox" --save-prompt
```

## 代理配置

```bash
# 命令行指定
aigc-cli image --prompt "..." --http-proxy "http://127.0.0.1:7890"

# 环境变量（支持 HTTP_PROXY / HTTPS_PROXY / ALL_PROXY / NO_PROXY）
export HTTP_PROXY="http://127.0.0.1:7890"

# SOCKS5
aigc-cli image --prompt "..." --http-proxy "socks5://127.0.0.1:1080"
```

## 优先级规则

**CLI 参数 > JSON 输入 > YAML 配置 > 代码默认值**

代理优先级：
**`--http-proxy` 参数 > `HTTP_PROXY` / `HTTPS_PROXY` 标准环境变量**
