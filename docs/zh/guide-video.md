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

## GIF 转换

`--gif` 支持两种场景：**AI 生成后自动转**，或**转换本地已有视频**（纯本地，不调 API、不消耗额度）。

### 生成后转 GIF

```bash
# 生成并转 GIF（宽度默认 160px）
aigc-cli video --prompt "一个人做俯卧撑" --gif

# 指定 GIF 宽度
aigc-cli video --prompt "一个人做俯卧撑" --gif --gif-width 320
```

### 转换本地已有视频

```bash
# 本地视频直接转 GIF（无需 prompt、不调 API）
aigc-cli video --gif -i 俯卧撑.mp4              # → 俯卧撑_160px.gif

# 指定宽度 / 自定义输出
aigc-cli video --gif -i clip.mp4 --gif-width 320
aigc-cli video --gif -i clip.mov --gif-width 0  # 0 = 保持原尺寸

# 裁掉四周边缘：每边裁掉 40px
aigc-cli video --gif -i org.mp4 --crop-margin 40

# 只裁上下边缘（CSS margin 简写：上下,左右）
aigc-cli video --gif -i org.mp4 --crop-margin 40,0

# 只裁底部一条（顺序：上,右,下,左）
aigc-cli video --gif -i org.mp4 --crop-margin 0,0,40,0
```

**说明：**
- 依赖系统 **ffmpeg**（须在 PATH），缺失时会提示安装方式。
- 转换参数已固化：`fps=6`、调色板 `max_colors=128`、`dither=none`（AI 视频平滑渐变下无抖动脉冲），日常只需调 `--gif-width`。
- 转换时会把实际执行的 ffmpeg 命令打印到 stdout，可直接复制复现。
- 高级用户可用 `--ffmpeg-flags` 追加额外参数（追加在 GIF filter 之后）。
- 生成路径下 `--gif` 同时作用于主生成、VEO3 Remix（`--remix`）和 `--job-id` 恢复三条路径。
- 本地转换的触发条件：`--gif` + `-i/--image-url` 指定本地视频文件 + **未指定 `--prompt`**。`-i` 是 `--image-url` 的简写（与 `image` 命令一致）。

## 边缘裁剪（Edge Crop）

`--crop-margin` 可以**单独使用**（不需要 `--gif`），重新编码视频裁掉四周边缘。**原视频始终保留**，输出新文件 `<stem>_crop.mp4`（与输入同目录）。只裁指定的边——例如 `40,0` 只裁上下 40px，左右保持不动。

```bash
# 裁剪本地视频（无 prompt、不调 API）：org.mp4 → org_crop.mp4
aigc-cli video --crop-margin 40 -i org.mp4
aigc-cli video --crop-margin 40,0 -i org.mp4        # 只裁上下边缘

# AI 生成后自动裁剪（保留原视频）
aigc-cli video --prompt "一个人做俯卧撑" --crop-margin 40
aigc-cli video --prompt "..." --crop-margin 0,0,40,0  # 只裁底部一条
```

**说明：**
- `--crop-margin` 支持 CSS margin 简写（逗号分隔）：1 个值=四边、2 个值=上下,左右、4 个值=上,右,下,左。精确按指定边裁掉像素，其他边不做额外裁切。`ffprobe`（随 ffmpeg 一起安装）用于校验裁边是否超过源视频尺寸。
- 单独裁剪是纯本地 ffmpeg 重编码（H.264），不调 API、不消耗额度。
- `--crop-margin` 同样作用于 VEO3 Remix（`--remix`）和 `--job-id` 恢复路径。

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
| `--image-url` | `-i` | 参考图片 URL 或本地文件（可重复）；配合 `--gif` 且无 `--prompt` 时转换本地视频 |
| `--first-frame` | | 首帧图片 |
| `--last-frame` | | 尾帧图片 |
| `--video-url` | | 参考视频 URL（可重复） |
| `--audio-url` | | 参考音频 URL（可重复） |
| `--json` | | JSON 输入（文件、字符串或 `-` 表示 stdin） |
| `--tool` | | 工具（如 `web_search`，可重复） |
| `--output` | | 下载目录（默认当前目录） |
| `--save-prompt` | | 保存 prompt 到 `video_{task_id}.md` |
| `--gif` | | 生成后把视频转成 GIF，或配合 `-i/--image-url` 转换本地视频（需 ffmpeg 在 PATH） |
| `--gif-width` | | GIF 输出宽度（px），高度自动等比取偶，默认 `160` |
| `--crop-margin` | | 裁掉四周边缘：无 prompt 时裁剪本地视频（`-i file`，保留原文件）；有 prompt 时裁剪 AI 生成的视频（保留原视频）。CSS margin 简写：`40`=四边、`40,0`=上下,左右、`40,30,20,10`=上,右,下,左 |
| `--ffmpeg-flags` | | 追加额外 ffmpeg 参数（高级逃生门，追加在 GIF filter 之后） |
| `--verbose` | `-v` | 显示请求 JSON 和完整响应（全局 flag） |

> ⚠️ **agnès 视频 `--resolution` 格式不同**：agnès（`agnes-video-2.5-flash` 等）要求 `720P`/`960P`/`2K`（大写 P），不能用 `480p`/`720p`（小写 p）。CLI 默认 `480p` 会被映射为 `720P`。详见 [agnès 视频文档](https://agnes-ai.com/zh-Hans/docs/agnes-video-v20)。

### agnès 图生视频

agnès 没有图片上传端点，本地图片会自动转为 base64 data URI 内嵌。支持三种模式：

| 模式 | 说明 | CLI 用法 |
|---|---|---|
| `text` | 纯文本生成（默认） | `--prompt "..."` |
| `reference` | 图片/音频参考 | `-i image1.jpg -i image2.jpg`（最多 5 张） |
| `keyframe` | 首尾帧控制 | `--first-frame img1.jpg --last-frame img2.jpg` |

```bash
# 图生视频：本地图片自动转 data URI
aigc-cli video -i photo.jpg --prompt "让照片中的人动起来" --gif

# 首尾帧
aigc-cli video --first-frame start.jpg --last-frame end.jpg --prompt "从首帧过渡到尾帧"
```

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

## 深度转换

深度转换已迁移到独立的 **`aigc-cli depth`** 命令，同时支持视频和单张图片转换。完整指南、参数和模型表（Depth Anything V2）见 [guide-depth.md](guide-depth.md)。

```bash
# 视频转灰度深度视频
aigc-cli depth -i input.mp4

# 首次使用
aigc-cli depth init
```

