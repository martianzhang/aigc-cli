---
name: aigc-cli-openrouter-video
description: Use "aigc-cli video" with OpenRouter to generate videos via the dedicated video API. Supports text-to-video, image-to-video (first frame), aspect ratio, resolution, and duration control. Uses async submit → poll → download pattern.
---

# aigc-cli-openrouter-video

通过 `aigc-cli video` 调用 OpenRouter 的专用视频 API（`POST /api/v1/videos`）生成视频。支持 Google Veo、Minimax 等视频模型。

**自动兼容**：当 `--api-base` 或 `OPENAI_BASE_URL` 指向 `openrouter.ai` 时，工具自动使用 OpenRouter 视频 API（而不是 APIMart 异步任务模式）。

## 前置条件

1. 项目已安装 `aigc-cli`（`go install` 或 `make build`）
2. 已配置 OpenRouter API Key：
   ```bash
   export OPENAI_API_KEY="sk-or-xxx"
   export OPENAI_BASE_URL="https://openrouter.ai/api/v1"
   ```
3. 或在 `~/.config/aigc-cli/config.yaml` 中配置：
   ```yaml
   api_key: "sk-or-xxx"
   base_url: "https://openrouter.ai/api/v1"
   ```

## 何时使用

- 用户需要根据文本描述生成视频（走 OpenRouter）
- 用户需要图生视频（首帧图片）
- 用户需要使用 Google Veo、Minimax 等 OpenRouter 支持的视频模型
- 用户需要指定分辨率、宽高比、时长
- 用户已经配置了 OpenRouter API Key

## 使用流程

### 1. 基本文生视频

```bash
# 设置 OpenRouter
export OPENAI_API_KEY="sk-or-xxx"
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"

# 生成视频（自动识别 OpenRouter，使用视频 API）
aigc-cli video --prompt "a golden retriever playing fetch on a sunny beach" \
  --model "google/veo-3.1"
```

提交后自动轮询（30 秒间隔），直到生成完成并下载视频。

### 2. 指定参数

```bash
aigc-cli video \
  --prompt "City nightscape timelapse" \
  --model "google/veo-3.1" \
  --resolution "720p" \
  --size "16:9" \
  --duration 8
```

### 3. 图生视频

```bash
aigc-cli video \
  --prompt "The kitten stands up and walks toward the camera" \
  --model "google/veo-3.1" \
  --image-url https://example.com/cat.jpg
```

### 4. 生成带音频的视频

```bash
aigc-cli video \
  --prompt "A man speaks to the camera: Hello everyone" \
  --model "minimax/video" \
  --generate-audio
```

## 常用 OpenRouter 视频模型

| 模型 ID | 说明 |
|---|---|
| `google/veo-3.1` | Google Veo 3.1 |
| `google/veo-3.0` | Google Veo 3.0 |
| `minimax/video` | MiniMax 视频模型 |

使用 `aigc-cli models` 查看完整模型列表。

## 视频生成流程

```
1. POST /api/v1/videos  → 返回 job_id + polling_url  (立即返回)
2. GET  {polling_url}   → 每 30 秒轮询一次           (30秒-几分钟)
3. GET  {unsigned_url}  → 下载 MP4 视频              (自动保存)
```

## 调试技巧

```bash
# 查看请求详情
aigc-cli video --prompt "test" --model "google/veo-3.1" --duration 4 -v

# Dry-run 查看 curl
aigc-cli video --prompt "test" --model "google/veo-3.1" --dry-run

# 指定输出目录
aigc-cli video --prompt "test" --model "google/veo-3.1" --output ./downloads
```

## 注意事项

- 视频生成是异步的，通常需要 30 秒到几分钟
- 提交后自动轮询，每 30 秒检查一次状态，最长等待 5 分钟
- 视频自动下载到当前目录（或用 `--output` 指定目录）
- `--generate-audio` 会增加处理时间
- 不同模型支持的参数（分辨率、时长、宽高比）不同，请参考 OpenRouter 文档
- 首次使用建议 `--dry-run` 确认请求参数
- 不要多次调用 API 重复测试，避免产生不必要的费用
