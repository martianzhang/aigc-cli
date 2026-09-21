# 视频生成

支持文生视频、图生视频、首尾帧、参考视频、音频视频、VEO3 Remix 续拍等模式。

## 自动兼容

根据 `base_url` 自动选择视频 API：

| Provider | 接口 | 模式 | 参考来源 |
|---|---|---|---|---|
| APIMart | `POST /v1/videos/generations` | 异步 task → poll → download | [APIMart Docs](https://docs.apimart.ai/en) |
| OpenRouter | `POST /v1/videos` | 异步 submit → poll → download | [OpenRouter Video](https://openrouter.ai/docs/guides/overview/multimodal/video-generation) |
| 云雾 Yunwu | `POST /v1/video/create` + `GET /v1/video/query?id=` | 异步 submit → poll → download | 云雾 API 文档 |
| Pollinations | `GET /video/{prompt}` | 同步，直接返回 MP4 | [Pollinations Docs](https://gen.pollinations.ai/docs) |
| 本地模型（Ollama / LocalAI 等） | ❌ 不支持 | — | 当前无本地开源方案支持视频生成 |
| 其他 | ❌ 不支持 | — | — |

当 `base_url` 包含 `openrouter.ai` 时，自动切换到 OpenRouter 视频 API。
当 `base_url` 指向 `pollinations.ai` 时，自动使用其同步媒体端点 `GET /video/{prompt}`。
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

## JSON 输入：原样转发 + 显式参数覆盖

**不带任何参数时**，`--json` 的原文字节会**原样**发往该 provider 的真实端点——CLI 不改名、不翻译、不归一化，也**不注入默认值**，因此厂商新参数无需改代码即可调试。

当 `--json` 与 CLI 参数同时出现时，只有你**显式指定**的参数会覆盖 body 中对应的键，其余键（包括 CLI 未建模的厂商私有键）原样保留：

```bash
# request.json 里的 generate_audio 被 flag 覆盖，其余键（如 aspect_ratio）一并保留
aigc-cli video --provider openrouter --json request.json --generate-audio
```

```bash
aigc-cli video --provider openrouter --json '{
  "model": "google/veo-3.1",
  "prompt": "a dog running",
  "aspect_ratio": "16:9",
  "generate_audio": true
}'
```

> 💡 参数到 JSON 键的映射沿用常规的 flag→字段映射（如 `--generate-audio` → `generate_audio`）。body 必须是 JSON **对象**；`--json` 支持内联字符串、文件路径或 `-`（stdin），路径文件不存在时明确报 `file not found: <路径>`。

> 💡 行为类参数永远不写进 body：`--dry-run`、`--preview`、`--provider`、`--api-key`、`--api-base`、`--http-proxy`、`--output`、`--verbose`、`--timeout`、`--config`、`--print-config`，以及 video 专有的 `--remix`、`--raw`、`--task-id`、`--job-id`、`--gif`、`--mp4`、`--crop-margin`、`--ffmpeg-flags`。

各 provider 的 `--json` 实际端点与原生形状不同（CLI 不会替你转换）：

| Provider | 端点 |
|---|---|
| OpenRouter | `POST {base}/videos` |
| Agnes | `POST {base}/videos` |
| Yunwu | `POST {base}/video/create` |
| Pollinations | `GET {pollinations 根}/video/{prompt}`（GET，无请求体） |
| APIMart / 通用 OpenAI 兼容 | `POST {base}/videos/generations` |

> 💡 用 `--flag`（`--prompt` / `--size` / `--duration` 等）时行为不变，CLI 仍按 provider 映射字段并补默认值；JSON 原样保留只针对 `--json` 未涉及的那些键。

> 💡 `--dry-run` 与 `--verbose` 打印的是**真实端点与真实请求体**（含上表的 provider 端点），可直接用来验证新参数。上传型 provider（APIMart、Yunwu）会先打印每个本地参考图的 multipart 上传 curl，再打印生成 curl，图片值用 `<UPLOAD_URL_n>` 占位。

## 参考图怎么发：上传 vs 内嵌 data URI

本地参考图（`--image-url` / `--first-frame` / `--last-frame`）的发送方式取决于 provider：上传型先传文件再引用返回的 URL，内嵌型直接编码成 `data:image/<mime>;base64,...`。远程 `https://` URL 和数据 URI 一律原样透传。

| 处理方式 | Provider | 预览里的图片值 |
|---|---|---|
| 上传（先传文件，再引用 URL） | APIMart、Yunwu（通用 OpenAI 兼容中转同样走此路径） | `<UPLOAD_URL_0>`、`<UPLOAD_URL_1>` … |
| 内嵌 data URI | OpenRouter、Agnes | `data:image/png;base64,...` |

### 上传型（APIMart、Yunwu）：`--dry-run` 打印上传 curl + 生成 curl

CLI 先对**每个本地参考图**发一次 multipart 上传，拿到公网 URL 后再发生成请求。`--dry-run` 如实打印这一串调用：N 个本地图片对应 N 条上传 curl，最后一条是生成 curl，生成 curl 里的图片值就是占位符 `<UPLOAD_URL_n>`。

```bash
aigc-cli video --base-url "https://api.apimart.ai" \
  --model "veo3.1-fast" \
  --prompt "从白天过渡到夜晚" \
  --first-frame day.jpg --last-frame night.jpg --dry-run
```

```bash
# 输出（API Key 已脱敏）
curl -X POST https://api.apimart.ai/v1/uploads/images \
  -H "Authorization: Bearer ...xxxx" \
  -F "file=@day.jpg"
curl -X POST https://api.apimart.ai/v1/uploads/images \
  -H "Authorization: Bearer ...xxxx" \
  -F "file=@night.jpg"
curl -X POST https://api.apimart.ai/v1/videos/generations \
  -H "Authorization: Bearer ...xxxx" \
  -H "Content-Type: application/json" \
  -d '{"model":"veo3.1-fast","prompt":"从白天过渡到夜晚","image_with_roles":[{"url":"<UPLOAD_URL_0>","role":"first_frame"},{"url":"<UPLOAD_URL_1>","role":"last_frame"}]}'
```

`--dry-run` 只做预览：**不发网络请求，也不会上传**。真实执行时占位符会被上传返回的 URL 替换。远程 URL 不触发上传，直接留在请求体里。用 `--image-url`（不带首尾帧角色）时，图片值落在 `image_urls` 数组里，同样是 `<UPLOAD_URL_n>` 占位。

### 内嵌型（无上传端点）

| Provider | 本地参考图落在 | 预览正文里的形状 |
|---|---|---|
| OpenRouter | `frame_images[]` | `{"type":"image_url","image_url":{"url":"data:image/png;base64,..."},"frame_type":"first_frame"}` |
| Agnes | `images[]`（reference 模式）或 `first_frame` / `last_frame`（keyframe 模式） | `data:image/png;base64,...` |

### 已知限制

| 限制 | 说明 |
|---|---|
| Yunwu 首尾帧角色被拉平 | Yunwu 的请求体只有一个 `images[]` 数组，CLI 会把 `--first-frame` / `--last-frame` 的 URL 依次放进 `images[]`，**不携带首帧/尾帧角色信息**。 |
| OpenRouter 多张 `--image-url` 都标为首帧 | OpenRouter 的 `frame_images[]` 里，每个来自 `--image-url` 的条目都会写成 `"frame_type":"first_frame"`；只有 `--first-frame` / `--last-frame` 才会保留各自的角色。 |

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

## Pollinations 视频（自动适配）

当检测到 `base_url` 含 `pollinations.ai` 时，使用其同步媒体端点 `GET /video/{prompt}`，直接返回 MP4 字节：

```bash
# 文生视频（社区模型）
aigc-cli video --provider pollinations \
  --model "community/NamanSoni78/Seedance-2.5" \
  --prompt "a cat walking in a garden" --duration 4

# 图生视频（首帧）
aigc-cli video --provider pollinations \
  --model "community/NamanSoni78/Seedance-2.5" \
  --prompt "the cat walks forward" --image-url ./cat.jpg
```

**说明：**
- 端点在 API 根路径 `https://gen.pollinations.ai/video/{prompt}`（**不在 `/v1` 下**）；`model`、`duration`、`seed` 以 query 参数传递，首帧图片通过 `image` 参数传入（`--image-url` / `--first-frame` 都会映射到它）。
- **是否免费取决于模型价格，与「社区/官方」无关**：只有 `pricing` 为 0 的模型才免费（用 `GET /video/models` 或 `aigc-cli models --provider pollinations` 查看）。官方模型（`google/veo-3.1-fast`、`bytedance/seedance-*`、`alibaba/wan-*` 等）标记 `paid_only: true`，**只能使用付费 pollen**，否则返回 `402 Insufficient balance`。**社区模型并不天然免费**：例如 `community/NamanSoni78/Seedance-2.5` 定价 `completionVideoSeconds: 0.25`，会消耗 pollen；只有像 `community/ZapGaming/failure-reel-v1` 这种价格为 0 的社区模型才不扣费。
- Pollinations 的视频端点**不接受 `--resolution`**（多数模型会返回 400），CLI 已自动不转发该参数；`--size` 同样不适用。
- 生成为同步调用，耗时通常 1–3 分钟；pollinations 在客户端断开后仍会继续生成，超时后**重发同一请求**即可（相同参数会命中缓存，不重复计费）。

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

## 媒体转 MP4

`--mp4` 把本地媒体文件（GIF / WebP / APNG / MOV / MKV / WebM / AVI 等任意 ffmpeg 可解码格式）转成 MP4（H.264 + yuv420p，兼容性最好）。纯本地，不调 API、不消耗额度。

```bash
# GIF 转 MP4：anim.gif → anim.mp4
aigc-cli video --mp4 -i anim.gif

# MOV / WebM 等其他格式，音轨会自动保留
aigc-cli video --mp4 -i clip.mov

# 先裁边再转（输出保持原分辨率）
aigc-cli video --mp4 -i anim.gif --crop-margin 10
```

**说明：**
- 依赖系统 **ffmpeg**（须在 PATH），缺失时会提示安装方式。
- 输出 `<stem>.mp4`（与输入同目录），**原文件保留**，不覆盖输入。
- 保持原分辨率（宽高自动取偶，H.264 要求偶数尺寸）；带透明通道的 GIF/WebP 会压成黑底。
- 源带音轨时自动保留；源无音轨则不产出音轨。
- 当输出会与输入为同一文件（如直接对 `.mp4` 转换）时直接报错，不覆盖输入。
- 转换时会把实际执行的 ffmpeg 命令打印到 stdout，可用 `--ffmpeg-flags` 追加额外参数。
- 与 `--gif` 互斥；只处理本地文件（`-i file` + 无 `--prompt`）。

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
| `--resolution` | `-r` | 分辨率：`480p`、`720p`、`1080p`，默认 `480p`（Pollinations 不使用此参数） |
| `--generate-audio` | `-a` | 生成 AI 音频 |
| `--dry-run` | | 打印等价 curl（上传型 provider 会先打印每个本地参考图的上传 curl），不调用 API |
| `--seed` | | 随机种子，用于复现 |
| `--return-last-frame` | | 返回最后一帧用于续拍 |
| `--image-url` | `-i` | 参考图片 URL 或本地文件（可重复）；配合 `--gif`/`--mp4` 且无 `--prompt` 时转换本地文件 |
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
| `--mp4` | | 把本地媒体文件（GIF/WebP/MOV 等）转成 MP4（`-i file`，需 ffmpeg；与 `--gif` 互斥） |
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

**Pollinations 视频**
- 同步生成，默认超时 600 秒
- 超时后服务端仍继续生成，用**完全相同的参数**重发请求即可（命中缓存，不重复计费）

**建议**：视频生成耗时长，推荐使用 APIMart 或 OpenRouter 的异步模式以获得可恢复能力。

## 深度转换

深度转换已迁移到独立的 **`aigc-cli depth`** 命令，同时支持视频和单张图片转换。完整指南、参数和模型表（Depth Anything V2）见 [guide-depth.md](guide-depth.md)。

```bash
# 视频转灰度深度视频
aigc-cli depth -i input.mp4

# 首次使用
aigc-cli depth init
```

