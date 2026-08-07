# 视频生成

支持文生视频、图生视频、首尾帧、参考视频、音频视频、VEO3 Remix 续拍等模式。

## 自动兼容

根据 `base_url` 自动选择视频 API：

| Provider | 接口 | 模式 | 参考来源 |
|---|---|---|---|---|
| APIMart | `POST /v1/videos/generations` | 异步 task → poll → download | [APIMart Docs](https://docs.apimart.ai/en) |
| OpenRouter | `POST /v1/videos` | 异步 submit → poll → download | [OpenRouter Video](https://openrouter.ai/docs/guides/overview/multimodal/video-generation) |
| 云雾 Yunwu | `POST /v1/video/create` + `GET /v1/video/query?id=` | 异步 submit → poll → download | 云雾 API 文档 |
| 本地模型（Ollama / LocalAI 等） | ❌ 不支持 | — | 当前无本地开源方案支持视频生成 |
| 其他 | ❌ 不支持 | — | — |

当 `base_url` 包含 `openrouter.ai` 时，自动切换到 OpenRouter 视频 API。
当 `base_url` 指向 `localhost` / `127.0.0.1` 时，视频生成不可用 — 当前没有开源本地方案支持视频生成（Ollama 和 LocalAI 均未实现视频生成端点）。

## 基本用法

```bash
# 文生视频
aigc-cli video --prompt "A kitten yawning at the camera"

# --prompt 不传时默认读 stdin
echo "A kitten yawning" | aigc-cli video
aigc-cli video < prompt.txt

# 指定分辨率及时长
aigc-cli video --prompt "City nightscape" --resolution 720p --duration 8

# 图生视频（首帧）
aigc-cli video --prompt "The kitten walks toward the camera" --image-url ./cat.jpg

# 首尾帧过渡
aigc-cli video --prompt "Transition from day to night" \
  --first-frame day.jpg --last-frame night.jpg

# 生成带音频的视频
aigc-cli video --prompt "A man speaks to the camera" --generate-audio

# 参考视频 + 参考音频
aigc-cli video --prompt "A person speaking" \
  --video-url ./reference.mp4 --audio-url ./speech.wav

# JSON 输入
aigc-cli video --json request.json
```

## VEO3 Remix（视频续拍）

> ⚠️ 仅 **VEO3** 系列模型支持 remix，不是所有视频模型都有此功能。

将已生成的视频从 8 秒**续拍到 15 秒**。模型必须与原始视频一致。

```bash
# 基本续拍
aigc-cli video --remix \
  --task-id task_xxx \
  --model veo3.1-fast \
  --prompt "The cat continues running on the grass"

# 只返回续拍部分（不包含原视频）
aigc-cli video --remix \
  --task-id task_xxx \
  --model veo3.1-quality \
  --prompt "keep dancing" \
  --raw

# 指定分辨率
aigc-cli video --remix \
  --task-id task_xxx \
  --model veo3.1-fast \
  --prompt "butterflies fly into the distance" \
  --resolution 1080p

# 更换比例
aigc-cli video --remix \
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

## OpenRouter 视频（自动适配）

当检测到 OpenRouter 时，使用专用视频 API（`POST /v1/videos`）：

```bash
# 文生视频
aigc-cli video --prompt "A golden retriever playing fetch" \
  --model "google/veo-3.1"

# 图生视频（首帧）
aigc-cli video --prompt "The dog runs toward the camera" \
  --model "google/veo-3.1" \
  --image-url https://example.com/dog.jpg

# 指定参数
aigc-cli video --prompt "City timelapse" \
  --model "google/veo-3.1" \
  --resolution 720p --duration 8
```

### 任务持久化（--job-id）

OpenRouter 视频生成是异步的（30 秒到几分钟）。提交后自动保存 job 信息，超时或断线后可重新拉取：

```bash
# 提交视频任务（自动保存 job 文件）
aigc-cli video --prompt "A kitten walking" --model "google/veo-3.1"
# → Job info saved. Resume later with: --job-id vid_xxx

# 断了之后重新拉取下载
aigc-cli video --job-id vid_xxx
```

Job 文件保存在 `video_job_{jobId}.json`，内含 `polling_url`、`model`、`prompt`、`created_at` 信息。

### 常用 OpenRouter 视频模型

| 模型 ID | 说明 |
|---|---|
| `google/veo-3.1` | Google Veo 3.1 |
| `google/veo-3.0` | Google Veo 3.0 |
| `minimax/video` | MiniMax 视频模型 |

使用 `aigc-cli models --type video`（免认证）查看完整列表。

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

## 超时处理

视频生成耗时较长（通常 30 秒到几分钟），超时处理方式取决于 provider：

**同步中转（部分第三方）**
- 默认超时 600 秒（10 分钟）
- 超时后无法恢复，需要重新生成
- 可通过 `--timeout` 增加：
  ```bash
  aigc-cli video --prompt "..." --timeout 900
  ```

**APIMart 异步模式**
- 超时后视频仍在后端渲染，不会丢失
- 使用 `task` 命令查询结果：
  ```bash
  aigc-cli task <task-id>
  ```

**OpenRouter 视频**
- 提交后返回 Job ID + polling_url，持久化到 `video_job_{id}.json`
- 超时后可用 `--job-id` 恢复：
  ```bash
  aigc-cli video --job-id <job-id>
  ```

**建议**：视频生成耗时长，推荐使用 APIMart 或 OpenRouter 的异步模式以获得可恢复能力。

## 深度转换（--convert-to-depth）

把任意视频转成**灰度深度视频**：画面中像素亮度表示相对距离（近亮远暗），去掉纹理、只保留动作和空间结构。这是深度引导 / 控制视频式图生视频（如 Wan VACE、Kling 动作控制、Vidu 参考生视频）的标准输入：把深度视频当动作/空间参考 + 上传一张参考照片，AI 就能生成"保留原动作节奏、换成参考照片内容"的新视频。

深度估计由本地 **Depth Anything V2** ONNX 模型完成（默认 `depth-anything-v2-small`，Apache-2.0），无需 API Key、无需联网。

```bash
# 首次使用：安装 ONNX Runtime + 深度模型（默认 small）
aigc-cli video init

# 转换整个视频
aigc-cli video --convert-to-depth -i input.mp4

# 只转换一段时间范围（--end-time 单独用 = 转前 N 秒）
aigc-cli video --convert-to-depth -i input.mp4 --start-time 00:01:00 --end-time 00:01:30

# 换模型 / 反转方向
aigc-cli video --convert-to-depth -i input.mp4 --depth-model depth-anything-v2-large --invert

# 自定义 ffmpeg 编码参数（追加在默认参数后，同名参数覆盖默认值）
aigc-cli video --convert-to-depth -i input.mp4 --encode-args "-crf 28 -preset slow"

# 只打印将要执行的 ffmpeg 命令，不实际转换
aigc-cli video --convert-to-depth -i input.mp4 --dry-run
```

输出自动命名为 `<输入文件名>_depth.mp4`（H.264、`yuv420p`、faststart），保存在输出目录（`--output`，默认当前目录）。

### 深度转换参数

| 参数 | 说明 | 默认 |
|---|---|---|
| `--convert-to-depth` | 开启深度转换模式 | off |
| `--input` / `-i` | 输入视频文件 | 必填 |
| `--start-time` | 开始时间（`SS`、`MM:SS`、`HH:MM:SS`） | 视频开头 |
| `--end-time` | 结束时间；单独用 = 转前 N 秒 | 视频结尾 |
| `--depth-model` | 模型：`depth-anything-v2-small` / `-base` / `-large`（别名：`small`、`base`、`large`） | `depth-anything-v2-small` |
| `--depth-size` | 推理分辨率（短边，14 对齐）。默认 280（快，深度结构清晰）；追求更高质量用 `378` 或 `518` | `280` |
| `--invert` | 反转深度方向（近暗远亮） | off |
| `--no-smooth` | 关闭时序平滑（开启时减轻闪烁） | off（平滑开） |
| `--keep-audio` | 保留原视频音轨 | off |
| `--encode-args` | 追加到 ffmpeg 编码命令的自定义参数，**追加在默认参数之后**——同名参数会覆盖默认值（后者生效）。例：`"-crf 28"`（更小文件）、`"-preset slow"`（更优压缩）、`"-crf 28 -preset slow"` | CRF 23，preset medium |

> **模型与许可证**：`depth-anything-v2-small` 为 Apache-2.0（可商用，默认）。`depth-anything-v2-base` 和 `-large` 为 CC-BY-NC-4.0（非商用），需要显式下载：`aigc-cli video init --model depth-anything-v2-base`。

### 依赖

- **ffmpeg**（aigc-cli 不捆绑）：需在 PATH 中。各平台安装方式见 [guide-ffmpeg.md](guide-ffmpeg.md)。
- **ONNX Runtime + 深度模型**：`aigc-cli video init` 一键安装。

### 提示

- 深度推理约 **3 帧/秒**（CPU）：10 秒 30fps 视频约需 2 分钟。建议先用 `--end-time` 转换一小段样片，调好 `--invert` / `--no-smooth` 后再转换全片。
- 输出默认不带音频（深度视频是动作参考）；如需保留原声用 `--keep-audio`。
- `--dry-run` 会打印抽帧和编码两条 ffmpeg 命令，方便你查看或手动调整参数。

