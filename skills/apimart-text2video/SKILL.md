---
name: apimart-text2video
description: Use "apimart-cli video" to generate videos via the APIMart doubao-seedance-2.0 API. Supports text-to-video, image-to-video, first/last frame, reference video/audio, and audio-enabled video. Automatically polls task and downloads videos.
---

# apimart-text2video

通过 `apimart-cli video` 调用 APIMart doubao-seedance-2.0 API 生成视频。提交任务后自动轮询完成并下载视频到当前目录。

## 前置条件

1. 项目已安装 `apimart-cli`（`go install` 或 `make build`）
2. 已配置 API Key（`~/.config/apimart/config.yaml` 或 `APIMART_API_KEY` 环境变量）
   - 视频默认参数在 `defaults.video` 下配置

## 何时使用

- 用户需要根据文本描述生成视频
- 用户需要上传图片生成视频（图生视频）
- 用户需要首帧 / 尾帧过渡动画
- 用户需要参考视频进行风格迁移
- 用户需要带音频的视频

## 使用流程

### 1. 基本文生视频

```bash
# 直接传提示词
apimart-cli video --prompt "A kitten yawning at the camera"

# --prompt 不传时默认读 stdin
echo "A cat walking" | apimart-cli video
apimart-cli video < prompt.txt
```

提交后自动轮询，任务完成即下载视频到当前目录。

### 2. 指定分辨率和时长

```bash
apimart-cli video \
  --prompt "City nightscape timelapse" \
  --resolution 720p \
  --duration 8 \
  --size "16:9"
```

### 3. 图生视频（首帧）

上传一张图片作为视频的第一帧：

```bash
apimart-cli video \
  --prompt "The kitten stands up and walks toward the camera" \
  --image-url ./cat.jpg
```

支持本地文件（自动上传）和远程 URL。

### 4. 首尾帧过渡

分别指定第一帧和最后一帧，生成过渡动画：

```bash
apimart-cli video \
  --prompt "Transition from day to night" \
  --first-frame day.jpg \
  --last-frame night.jpg
```

### 5. 生成带音频的视频

```bash
apimart-cli video \
  --prompt "A man speaks to the camera: Hello everyone" \
  --generate-audio
```

### 6. 参考视频 + 参考音频

```bash
apimart-cli video \
  --prompt "Convert to anime style" \
  --video-url ./reference.mp4 \
  --audio-url ./background-music.wav
```

### 7. 续拍（返回最后一帧）

```bash
apimart-cli video \
  --prompt "The kitten continues walking" \
  --image-url ./prev_last_frame.png \
  --return-last-frame
```

### 8. JSON 输入

```bash
apimart-cli video --json '{
  "model": "doubao-seedance-2.0",
  "prompt": "A kitten yawning",
  "resolution": "720p",
  "duration": 5
}'
```

## 最经济配置

参考定价页 https://apimart.ai/zh/pricing

`doubao-seedance-2.0` 最低 **$0.0224/个**（480p，5秒）：

```bash
echo "A cat walking" | apimart-cli video --duration 4
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
apimart-cli video --prompt "..." --http-proxy "http://127.0.0.1:7890"

# 环境变量（自动识别）
export HTTP_PROXY="http://127.0.0.1:7890"
```

## Dry-run 调试

查看即将提交的 curl 命令，不实际调用 API：

```bash
apimart-cli video --prompt "test" --duration 4 --dry-run
```

## 注意事项

- 提交后自动轮询任务，最长等待 180 秒
- 视频默认时长 5 秒，支持 4-15 秒
- 视频自动下载到当前目录（或用 `--output` 指定目录）
- `--generate-audio` 会增加处理时间
- 不要多次调用 API 测试，避免产生不必要的费用
- 支持 `--first-frame` / `--last-frame` 分别指定首尾帧（无需同时使用）
