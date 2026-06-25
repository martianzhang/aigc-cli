# 图片生成

支持**同步模式**（OpenAI / OpenRouter 兼容，直接返回图片）和**异步任务模式**（APIMart，提交后轮询结果），自动根据 API 地址检测。

支持文生图、图生图、Inpainting 三种模式。

## 基本用法（文生图）

```bash
# 直接传提示词（APIMart 自动异步模式）
apimart-cli image --prompt "一只猫在星空下"

# OpenAI / OpenRouter 自动同步模式
apimart-cli image --base-url "https://openrouter.ai/api/v1" \
  --prompt "a cat"

# 从文件读取（自动识别文件路径）
apimart-cli image --prompt prompt.txt

# 从 stdin 读取
echo "赛博朋克城市夜景" | apimart-cli image
apimart-cli image < prompt.txt
```

## 参数

| 参数 | 短参 | 说明 | 适用 |
|---|---|---|---|
| `--prompt` | `-p` | 文本描述（自动识别文件/stdin） | 通用 |
| `--model` | `-m` | 模型名（必填，可通过 `defaults.image.model` 在配置文件中设置默认值） | 通用 |
| `--size` | `-s` | 宽高比，如 `16:9`、`1:1`，或像素如 `1024x1024` | 通用 |
| `--quality` | `-q` | 质量：`auto`、`low`、`medium`、`high` | 通用 |
| `--output-format` | `-f` | 输出格式：`png`、`jpeg`、`webp` | 通用 |
| `--n` | | 生成数量 1-4 | 通用 |
| `--style` | | 风格：`vivid`、`natural`（OpenAI 专用） | OpenAI |
| `--response-format` | | 响应格式：`url`、`b64_json` | OpenAI/OpenRouter |
| `--resolution` | `-r` | 分辨率档：`1k`、`2k`、`4k` | APIMart |
| `--background` | | 背景：`auto`、`opaque`、`transparent` | APIMart |
| `--moderation` | | 审核强度：`auto`、`low` | APIMart |
| `--output-compression` | | 压缩率 0-100（jpeg/webp） | APIMart |
| `--image-url` | | 参考图片 URL（可重复） | APIMart |
| `--mask-url` | | 蒙版图片 URL（inpainting） | APIMart |
| `--json` | | JSON 输入（文件、字符串或 `-` 表示 stdin） | 通用 |
| `--output` | | 下载目录（默认当前目录，支持相对/绝对路径） | 通用 |
| `--save-prompt` | | 保存 prompt 到 `image_{task_id}.md` | 通用 |
| `--verbose` | `-v` | 显示请求 JSON 和完整响应（全局 flag） | 通用 |
| `--mode` | | 强制指定模式：`auto`、`sync`、`async` | 通用 |
| `--dry-run` | | 打印 curl 不调用 API | 通用 |

### 模式自动检测规则

| base_url 包含 | 模式 | 说明 |
|---|---|---|
| `apimart.ai` / `apib.ai` / `aiuxu.com` / `aishuch.com` | async | APIMart 异步任务 |
| `openai.com` / `openrouter.ai` 或其他 | sync | OpenAI 兼容同步 |

可通过 `apimart-cli image --mode sync|async` 强制指定。

```bash
# 强制异步（即使连的是 OpenAI 兼容中转）
apimart-cli image --mode async --prompt "..."

# 强制同步（即使连的是 APIMart）
apimart-cli image --mode sync --prompt "..."
```

## 同步模式（OpenAI / OpenRouter）

图片直接返回，无需等待轮询，下载到当前目录：

```bash
apimart-cli image --base-url "https://openrouter.ai/api/v1" \
  --model "openai/dall-e-3" \
  --prompt "A cute cat" \
  --n 2 \
  --style vivid

# 输出示例：
# Created: 1712345678
# Image 1: https://.../image1.png
#   Revised prompt: A cute cat in a vibrant style
# Saved: image_sync_1712345678_0.png
```

## JSON 输入

构建完整的请求 JSON 传入：

```bash
# JSON 文件
apimart-cli image --json request.json

# JSON 字符串
apimart-cli image --json '{"prompt":"a red fox","n":4}'

# 从 stdin
cat request.json | apimart-cli image --json -
```

## 参考图生图（image-to-image，APIMart）

参考已有图片进行融合或编辑，支持本地文件（自动上传）和远程 URL：

```bash
# 本地文件（自动上传到 APIMart）
apimart-cli image \
  --prompt "把这张照片改成吉卜力风格" \
  --image-url ./my-photo.jpg

# 远程 URL
apimart-cli image \
  --prompt "融合两张参考图，保留主要轮廓" \
  --image-url "https://example.com/img1.png" \
  --image-url "https://example.com/img2.png"
```

## Grok Imagine 1.5 Edit（图片编辑，APIMart）

> ⚠️ 仅 **Grok Imagine 1.5 Edit** 模型支持此模式，不是所有图片模型都有 `--edit` 功能。

基于已有图片 + 文本描述进行编辑替换，支持背景替换、风格迁移等：

```bash
# 背景替换
apimart-cli image --edit \
  --prompt "把背景换成星空，保留主体" \
  --image-url ./photo.jpg

# 风格迁移
apimart-cli image --edit \
  --prompt "转换成赛博朋克风格" \
  --image-url ./img.png \
  --n 2

# 指定模型（不写默认 grok-imagine-1.5-edit-apimart）
apimart-cli image --edit \
  --model "grok-imagine-1.5-edit-apimart" \
  --prompt "Change the background to a starry sky" \
  --image-url "https://example.com/img.png"
```

### edit 模式说明

| 规则 | 说明 |
|---|---|
| `--edit` 开关 | 不带则走普通文生图/图生图流程 |
| `--image-url` | **必填**，至少 1 张源图 |
| `--model` | 不指定则默认 `grok-imagine-1.5-edit-apimart` |
| `--n` | 1-10（普通模式 1-4） |
| 模式 | **强制异步**，仅 APIMart 可用 |
| size/quality 等 | 编辑模式下不适用，自动跳过 |

## Inpainting（蒙版替换，APIMart）

提供原图和蒙版，替换指定区域：

```bash
# 本地文件自动上传
apimart-cli image \
  --prompt "把背景换成沙漠日落" \
  --image-url ./photo.png \
  --mask-url ./mask.png

# 远程 URL
apimart-cli image \
  --prompt "Replace background with desert sunset" \
  --image-url "https://example.com/photo.png" \
  --mask-url "https://example.com/mask.png"
```

> `--image-url` 和 `--mask-url` 仅在 APIMart 异步模式下可用。

## 最经济配置（APIMart）

参考 [APIMart 定价](https://apimart.ai/pricing)，`gpt-image-2-official` 最低 **$0.00144/张**：

```bash
apimart-cli image --prompt "..." \
  --size "3:1" \
  --resolution "1k" \
  --quality "low"
```

或写入 `~/.config/openai/config.yaml` 作为默认值。

## 输出格式建议

| 格式 | 适用场景 |
|---|---|
| PNG | 需要透明背景、后续编辑、对画质要求高 |
| JPEG | 日常使用、社交媒体分享、网页展示 |
| WebP | 网页使用、需要小文件体积、支持透明 |
