# 视频生成

支持文生视频、图生视频、首尾帧、参考视频、音频视频等模式。

## 基本用法

```bash
# 文生视频
apimart-cli video --prompt "A kitten yawning at the camera"

# --prompt 不传时默认读 stdin
echo "A kitten yawning" | apimart-cli video
apimart-cli video < prompt.txt

# 指定分辨率及时长
apimart-cli video --prompt "City nightscape" --resolution 720p --duration 8

# 图生视频（首帧）
apimart-cli video --prompt "The kitten walks toward the camera" --image-url ./cat.jpg

# 首尾帧过渡
apimart-cli video --prompt "Transition from day to night" \
  --first-frame day.jpg --last-frame night.jpg

# 生成带音频的视频
apimart-cli video --prompt "A man speaks to the camera" --generate-audio

# 参考视频 + 参考音频
apimart-cli video --prompt "A person speaking" \
  --video-url ./reference.mp4 --audio-url ./speech.wav

# JSON 输入
apimart-cli video --json request.json
```

## 参数

| 参数 | 短参 | 说明 |
|---|---|---|
| `--prompt` | `-p` | 视频内容描述 |
| `--model` | `-m` | 模型名（必填，可通过 `defaults.video.model` 在配置文件中设置默认值） |
| `--duration` | `-d` | 时长 4-15 秒，默认 5 |
| `--size` | `-s` | 宽高比：`16:9`、`9:16`、`1:1`、`4:3`、`3:4`、`21:9`、`adaptive` |
| `--resolution` | `-r` | 分辨率：`480p`、`720p`、`1080p`，默认 `480p` |
| `--generate-audio` | `-a` | 生成 AI 音频 |
| `--dry-run` | | 打印 curl 不调用 API |
| `--seed` | | 随机种子，用于复现 |
| `--return-last-frame` | | 返回最后一帧用于续拍 |
| `--image-url` | | 参考图片 URL（可重复） |
| `--first-frame` | | 首帧图片 |
| `--last-frame` | | 尾帧图片 |
| `--video-url` | | 参考视频 URL（可重复） |
| `--audio-url` | | 参考音频 URL（可重复） |
| `--json` | | JSON 输入（文件、字符串或 `-` 表示 stdin） |
| `--tool` | | 工具（如 `web_search`，可重复） |
