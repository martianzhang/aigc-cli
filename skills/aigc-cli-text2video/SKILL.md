---
name: aigc-cli-text2video
description: Use "aigc-cli video" to generate videos via the APIMart API (doubao-seedance-2.0, VEO3). Supports text-to-video, image-to-video, first/last frame, reference video/audio, audio-enabled video, VEO3 Remix (--remix), seed, web_search tool, and return-last-frame for continuation. Automatically polls task and downloads videos.
---

# aigc-cli-text2video

通过 `aigc-cli video` 调用 APIMart 视频 API 生成视频（doubao-seedance-2.0、VEO3 等）。支持 VEO3 Remix（`--remix`）视频续拍。提交任务后自动轮询完成并下载视频到当前目录。

支持 `--seed` 种子复现、`--tool web_search` 联网搜索、`--return-last-frame` 返回尾帧用于续拍。

调试参数：`--dry-run`（打印 curl）、`-v` / `--verbose`（打印请求 JSON）、`--save-prompt`（保存 prompt）。

## 前置条件

1. 项目已安装 `aigc-cli`（`go install` 或 `make build`）
2. 已配置 API Key（`~/.config/aigc-cli/config.yaml` 或 `OPENAI_API_KEY` 环境变量）
   - 视频默认参数在 `defaults.video` 下配置

## 何时使用

- 用户需要根据文本描述生成视频
- 用户需要上传图片生成视频（图生视频）
- 用户需要首帧 / 尾帧过渡动画
- 用户需要参考视频进行风格迁移
- 用户需要带音频的视频
- 用户需要 VEO3 视频续拍（8s→15s，`--remix` 模式）
- 用户需要联网搜索获取最新信息后生成视频（`--tool web_search`）
- 用户需要固定随机种子复现相同结果（`--seed`）
- 用户需要返回最后一帧用于后续续拍（`--return-last-frame`）

## 使用流程

### 1. 基本文生视频

```bash
# 直接传提示词
aigc-cli video --prompt "A kitten yawning at the camera"

# --prompt 不传时默认读 stdin
echo "A cat walking" | aigc-cli video
aigc-cli video < prompt.txt
```

提交后自动轮询，任务完成即下载视频到当前目录。

### 2. 指定分辨率和时长

```bash
aigc-cli video \
  --prompt "City nightscape timelapse" \
  --resolution 720p \
  --duration 8 \
  --size "16:9"
```

### 3. 图生视频（首帧）

上传一张图片作为视频的第一帧：

```bash
aigc-cli video \
  --prompt "The kitten stands up and walks toward the camera" \
  --image-url ./cat.jpg
```

支持本地文件（自动上传）和远程 URL。

### 4. 首尾帧过渡

分别指定第一帧和最后一帧，生成过渡动画：

```bash
aigc-cli video \
  --prompt "Transition from day to night" \
  --first-frame day.jpg \
  --last-frame night.jpg
```

### 5. 生成带音频的视频

```bash
aigc-cli video \
  --prompt "A man speaks to the camera: Hello everyone" \
  --generate-audio
```

### 6. 参考视频 + 参考音频

```bash
aigc-cli video \
  --prompt "Convert to anime style" \
  --video-url ./reference.mp4 \
  --audio-url ./background-music.wav
```

### 7. 续拍（返回最后一帧）

```bash
aigc-cli video \
  --prompt "The kitten continues walking" \
  --image-url ./prev_last_frame.png \
  --return-last-frame
```

### 8. VEO3 Remix（视频续拍 8s→15s）

> ⚠️ 仅 VEO3 系列模型支持，模型须与原始视频一致。

```bash
# 基本续拍
aigc-cli video --remix \
  --task-id task_xxx \
  --model veo3.1-fast \
  --prompt "The cat continues running on the grass"
```

`--remix` 模式下 `--task-id`、`--model`、`--prompt` 均为必填。

Remix 支持 `--raw` 参数：指定后只返回续拍的新片段（不包含原始视频）：
```bash
aigc-cli video --remix \
  --task-id task_xxx \
  --model veo3.1-fast \
  --prompt "keep going" \
  --raw \
  --resolution 1080p
```

### 9. JSON 输入

```bash
# JSON 字符串
aigc-cli video --json '{
  "model": "doubao-seedance-2.0",
  "prompt": "A kitten yawning",
  "resolution": "720p",
  "duration": 5
}'

# JSON 文件
aigc-cli video --json request.json

# stdin
cat request.json | aigc-cli video --json -
```

### 10. 种子复现

指定随机种子，保证相同参数生成相同结果：

```bash
aigc-cli video --prompt "A cat walking" --seed 42
```

### 11. 联网搜索

使用 `--tool` 参数启用工具：

```bash
aigc-cli video --prompt "根据最新新闻生成一段视频" --tool web_search
```

## 最经济配置

参考定价页 https://apimart.ai/zh/pricing

`doubao-seedance-2.0` 最低 **$0.0224/个**（480p，5秒）：

```bash
echo "A cat walking" | aigc-cli video --duration 4
```

或设入 config.yaml 作为全局默认值：

```yaml
defaults:
  video:
    model: "doubao-seedance-2.0"
    size: "16:9"
    resolution: "480p"
```

## 代理

```bash
# --http-proxy 参数（支持 http/https/socks5）
aigc-cli video --prompt "..." --http-proxy "http://127.0.0.1:7890"

# 环境变量（自动识别）
export HTTP_PROXY="http://127.0.0.1:7890"
```

## 全部参数一览

| 参数 | 功能 |
|---|---|
| `--prompt` | 视频描述文本（支持文件/stdin） |
| `--model` | 视频模型名 |
| `--duration` | 视频时长 4-15 秒 |
| `--size` | 宽高比：16:9, 9:16, 1:1, 4:3, 3:4, 21:9, adaptive |
| `--resolution` | 分辨率：480p, 720p, 1080p |
| `--seed` | 随机种子（复现相同结果） |
| `--generate-audio` | 生成 AI 音频 |
| `--return-last-frame` | 返回最后一帧（用于续拍） |
| `--image-url` | 参考图片（可重复，支持本地文件） |
| `--first-frame` | 首帧图片（本地文件自动上传） |
| `--last-frame` | 尾帧图片（本地文件自动上传） |
| `--video-url` | 参考视频（可重复） |
| `--audio-url` | 参考音频（可重复） |
| `--tool` | 工具类型（如 web_search，可重复） |

## 调试技巧

```bash
# Dry-run：打印 curl 命令，不实际调用
aigc-cli video --prompt "test" --duration 4 --dry-run

# 查看请求 JSON
aigc-cli video --prompt "test" -v

# 保存 prompt 到 video_{task_id}.md（后续可追溯）
aigc-cli video --prompt "A cat" --save-prompt

# 指定输出目录
aigc-cli video --prompt "cat" --output ./downloads
```

## 注意事项

- 提交后自动轮询任务，最长等待 180 秒
- 视频默认时长 5 秒，支持 4-15 秒
- 视频自动下载到当前目录（或用 `--output` 指定目录）
- `--generate-audio` 会增加处理时间
- `--remix` + `--raw` 只返回续拍片段（不含原始视频）
- `--tool web_search` 可让模型联网搜索后生成视频
- 支持 `--first-frame` / `--last-frame` 分别指定首尾帧（无需同时使用）
- 首次使用建议 `--dry-run` 先确认请求参数正确
