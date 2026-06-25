# 视频生成

支持文生视频、图生视频、首尾帧、参考视频、音频视频、VEO3 Remix 续拍等模式。

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

## VEO3 Remix（视频续拍）

> ⚠️ 仅 **VEO3** 系列模型支持 remix，不是所有视频模型都有此功能。

将已生成的视频从 8 秒**续拍到 15 秒**。模型必须与原始视频一致。

```bash
# 基本续拍
apimart-cli video --remix \
  --task-id task_xxx \
  --model veo3.1-fast \
  --prompt "The cat continues running on the grass"

# 只返回续拍部分（不包含原视频）
apimart-cli video --remix \
  --task-id task_xxx \
  --model veo3.1-quality \
  --prompt "keep dancing" \
  --raw

# 指定分辨率
apimart-cli video --remix \
  --task-id task_xxx \
  --model veo3.1-fast \
  --prompt "butterflies fly into the distance" \
  --resolution 1080p

# 更换比例
apimart-cli video --remix \
  --task-id task_xxx \
  --model veo3.1-fast \
  --prompt "continue" \
  --size "9:16"
```

### remix 模式参数

| 参数 | 说明 |
|---|---|
| `--remix` | 开启 VEO3 Remix 模式 |
| `--task-id` | **必填**，原始视频的 task_id |
| `--model` | **必填**，必须与原始视频的模型一致（`veo3.1-fast` / `veo3.1-quality`） |
| `--prompt` / `-p` | **必填**，续拍内容描述 |
| `--raw` | 只返回续拍部分，不含原视频 |
| `--size` / `-s` | 宽高比：`16:9`、`9:16` |
| `--resolution` / `-r` | 分辨率：`720p`（默认）、`1080p`、`4k` |

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
| `--output` | | 下载目录（默认当前目录） |
| `--save-prompt` | | 保存 prompt 到 `video_{task_id}.md` |
| `--verbose` | `-v` | 显示请求 JSON 和完整响应（全局 flag） |
