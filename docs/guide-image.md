# 图片生成

支持**同步模式**（OpenAI / OpenRouter 兼容，直接返回图片）和**异步任务模式**（APIMart，提交后轮询结果），自动根据 API 地址检测。

支持文生图、图生图、Inpainting 三种模式。

## 基本用法（文生图）

```bash
# 直接传提示词（APIMart 自动异步模式）
aigc-cli image --prompt "一只猫在星空下"

# OpenAI / OpenRouter 自动同步模式
aigc-cli image --base-url "https://openrouter.ai/api/v1" \
  --prompt "a cat"

# 从文件读取（自动识别文件路径）
aigc-cli image --prompt prompt.txt

# 从 stdin 读取
echo "赛博朋克城市夜景" | aigc-cli image
aigc-cli image < prompt.txt
```

## 参数

| 参数 | 短参 | 说明 | 适用 |
|---|---|---|---|---|
| `--prompt` | `-p` | 文本描述（自动识别文件/stdin） | 通用 |
| `--model` | `-m` | 模型名（可通过 `defaults.image.model` 或 `providers.{name}.model` 设默认值） | 通用 |
| `--provider` | | 命名 Provider 名称（覆盖 `defaults.image.provider`，见 `docs/config.example.yaml`） | 通用 |
| `--size` | `-s` | 宽高比，如 `16:9`、`1:1`，或像素如 `1024x1024` | 通用 |
| `--quality` | `-q` | 质量：`auto`、`low`、`medium`、`high` | 通用 |
| `--output-format` | `-f` | 输出格式：`png`、`jpeg`、`webp` | 通用 |
| `--compress` | `-z` | 压缩目标：`800KB`/`2MB`（目标大小）或 `85%`（固定 quality） | 通用 |
| `--n` | | 生成数量 1-4 | 通用 |
| `--style` | | 风格：`vivid`、`natural`（OpenAI 专用） | OpenAI |
| `--response-format` | | 响应格式：`url`、`b64_json` | OpenAI/OpenRouter |
| `--resolution` | `-r` | 分辨率档：`1k`、`2k`、`4k` | APIMart |
| `--background` | | 背景：`auto`、`opaque`、`transparent` | APIMart |
| `--moderation` | | 审核强度：`auto`、`low` | APIMart |
| `--output-compression` | | 压缩率 0-100（jpeg/webp） | APIMart |
| `--image-url` | `-i` | 图片输入：URL 或本地文件路径（可重复） | 通用 |
| `--mask-url` | | 蒙版图片 URL（inpainting） | APIMart |
| `--json` | | JSON 输入（文件、字符串或 `-` 表示 stdin） | 通用 |
| `--output` | | 下载目录（默认当前目录，支持相对/绝对路径） | 通用 |
| `--save-prompt` | | 保存 prompt 到 `image_{task_id}.md` | 通用 |
| `--verbose` | `-v` | 显示请求 JSON 和完整响应（全局 flag） | 通用 |
| `--mode` | | 强制指定模式：`auto`、`sync`、`async` | 通用 |
| `--dry-run` | | 打印 curl 不调用 API | 通用 |
| `--preview` | | 生成后自动用系统默认程序打开图片 | 通用 |

### 模式自动检测规则

| base_url 包含 | 模式 | 说明 |
|---|---|---|
| `apimart.ai` / `apib.ai` / `aiuxu.com` / `aishuch.com` | async | APIMart 异步任务 |
| `openai.com` / `openrouter.ai` 或其他 | sync | OpenAI 兼容同步 |
| `localhost` / `127.0.0.1` / `::1` | sync | 本地模型（自动豁免 API Key 检查） |

可通过 `aigc-cli image --mode sync|async` 强制指定。

```bash
# 强制异步（即使连的是 OpenAI 兼容中转）
aigc-cli image --mode async --prompt "..."

# 强制同步（即使连的是 APIMart）
aigc-cli image --mode sync --prompt "..."
```

## 本地生成

aigc-cli 支持连接本地运行的图片生成服务（Ollama / LocalAI 等），无需 API Key。aigc-cli 会自动检测 localhost 地址并跳过 Authorization 头。

### Ollama

Ollama 从 v0.5.0+ 开始实验性地支持 `/v1/images/generations` 端点：

```bash
# 先 pull 图片生成模型
ollama pull x/z-image-turbo           # 6B, Apache 2.0
ollama pull x/flux2-klein:4b          # 4B, 商业友好
ollama pull x/flux2-klein:9b          # 9B, FLUX 非商业许可

# 文生图
aigc-cli image --api-base "http://localhost:11434" \
  --model "x/z-image-turbo" \
  --prompt "A cute robot learning to paint"
```

> ⚠️ Ollama 仅支持 `response_format=b64_json` 格式，不支持 URL 格式。aigc-cli 会自动处理 base64 解码保存。

### LocalAI

LocalAI 也支持图片生成（多后端），与 aigc-cli 无缝对接：

```bash
# Docker 启动
docker run -p 8080:8080 localai/localai:latest

# 文生图
aigc-cli image --api-base "http://localhost:8080/v1" \
  --model "stabilityai/sd-turbo" \
  --prompt "a cat"
```

## 同步模式（OpenAI / OpenRouter）

图片直接返回，无需等待轮询，下载到当前目录：

```bash
aigc-cli image --base-url "https://openrouter.ai/api/v1" \
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

直接传入完整请求 JSON，绕过 CLI 参数解析。适合底层调试、脚本化调用和 CI/CD，所有参数在一个文件里管理：

```bash
# 使用项目提供的示例文件
aigc-cli image --json docs/example.json

# 自定义 JSON 文件
aigc-cli image --json request.json

# JSON 字符串（适合调试/快速验证）
aigc-cli image --json '{"prompt":"a red fox","n":4}'

# 从 stdin
cat request.json | aigc-cli image --json -
```

项目根目录 `docs/example.json` 提供了完整示例：

```json
{
  "model": "gpt-image-2-official",
  "prompt": "A serene mountain lake at sunrise, photorealistic",
  "size": "3:1",
  "resolution": "1k",
  "quality": "low",
  "output_format": "jpeg",
  "output_compression": 85,
  "n": 1
}
```

JSON 输入优先级高于 CLI 参数（参考[优先级规则](installation.md#优先级规则)），适合脚本化调用和 CI/CD 集成。

## 参考图生图（image-to-image，APIMart）

参考已有图片进行融合或编辑，支持本地文件（自动上传）和远程 URL：

```bash
# 本地文件（自动上传到 APIMart）
aigc-cli image \
  --prompt "把这张照片改成吉卜力风格" \
  --image-url ./my-photo.jpg

# 远程 URL
aigc-cli image \
  --prompt "融合两张参考图，保留主要轮廓" \
  --image-url "https://example.com/img1.png" \
  --image-url "https://example.com/img2.png"
```

## Grok Imagine 1.5 Edit（图片编辑，APIMart）

> ⚠️ 仅 **Grok Imagine 1.5 Edit** 模型支持此模式，不是所有图片模型都有 `--edit` 功能。

基于已有图片 + 文本描述进行编辑替换，支持背景替换、风格迁移等：

```bash
# 背景替换
aigc-cli image --edit \
  --prompt "把背景换成星空，保留主体" \
  --image-url ./photo.jpg

# 风格迁移
aigc-cli image --edit \
  --prompt "转换成赛博朋克风格" \
  --image-url ./img.png \
  --n 2

# 指定模型（不写默认 grok-imagine-1.5-edit-apimart）
aigc-cli image --edit \
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
aigc-cli image \
  --prompt "把背景换成沙漠日落" \
  --image-url ./photo.png \
  --mask-url ./mask.png

# 远程 URL
aigc-cli image \
  --prompt "Replace background with desert sunset" \
  --image-url "https://example.com/photo.png" \
  --mask-url "https://example.com/mask.png"
```

> `--image-url` 和 `--mask-url` 仅在 APIMart 异步模式下可用。

## 最经济配置（APIMart）

参考 [APIMart 定价](https://apimart.ai/pricing)，`gpt-image-2-official` 最低 **$0.00144/张**：

```bash
aigc-cli image --prompt "..." \
  --size "3:1" \
  --resolution "1k" \
  --quality "low"
```

或写入 `~/.config/aigc-cli/config.yaml` 作为默认值。

## 输出格式建议

| 格式 | 适用场景 |
|---|---|
| PNG | 需要透明背景、后续编辑、对画质要求高 |
| JPEG | 日常使用、社交媒体分享、网页展示 |
| WebP | 网页使用、需要小文件体积、支持透明 |

## 超时处理

图片生成可能因模型响应慢或网络问题超时。处理方式取决于 provider：

**同步模式（OpenAI / OpenRouter / 第三方中转）**
- 默认超时 180 秒
- 超时后无法恢复，需要重新生成
- 可通过 `--timeout` 增加超时时间：
  ```bash
  aigc-cli image --prompt "..." --timeout 300
  ```
- 或在配置文件中设置：`timeout: 300`

**异步模式（APIMart）**
- 超时后任务仍在后端运行，不会丢失
- 使用 `task` 命令查询结果：
  ```bash
  aigc-cli task <task-id>
  ```
- 任务完成时会自动下载结果

**建议**：如果频繁超时，优先考虑增加 `--timeout`；需要可恢复能力则使用 APIMart 异步模式。

## 本地压缩（--compress）

`--compress` 支持两种模式：**生成后自动压缩** 和 **纯本地压缩**（不走 API）。

### 路径 1：生成后自动压缩

图片生成完成后自动对结果进行压缩：

```bash
# 生成 JPEG，目标文件大小 500KB
aigc-cli image --prompt "猫" --compress 500KB -f jpg

# 生成 WebP，固定 quality 85
aigc-cli image --prompt "猫" --compress 85% -f webp

# 使用 YAML 配置默认值
# defaults:
#   image:
#     compress: 500KB
#     output_format: jpg
```

压缩结果示例输出：
```
  Compress image_1712345678_0.png: 2.3MB → 489.2KB (79% saved)
```

### 路径 2：纯本地压缩

无 `--prompt`、无 API 调用，直接压缩本地已有图片：

```bash
# 单张压缩，目标 256KB
aigc-cli image --compress 256KB -i photo.png

# 批量压缩多张 + 转 JPEG
aigc-cli image --compress 500KB -f jpg -i a.png -i b.png -i c.webp
```

纯本地模式下 `--image-url` 只接受本地文件路径，URL 会被跳过并给出警告。

### 参数值格式

| 格式 | 示例 | 行为 |
|---|---|---|
| `800KB` / `800kb` | `--compress 800KB` | 目标文件大小 800KB，自动选最高 quality |
| `2MB` / `2mb` | `--compress 2MB` | 目标文件大小 2MB |
| `85%` | `--compress 85%` | 固定 quality=85 编码 |

**自动 quality 推导**（指定目标大小时）：
1. 搜索范围 quality [50, 95]
2. 二分搜索（约 6 轮），每轮用 mid quality 试编码
3. 返回满足 ≤ target 的最高 quality
4. 若 quality=50 仍超 target，降 quality=30 尽力

### 边界情况

| 场景 | 行为 |
|---|---|
| 压缩后反比原图大 | 跳过，提示不处理 |
| 原图已 ≤ target | 跳过，提示已达标 |
| PNG + `-f jpg` | 正常执行（透明度丢失） |
| `--compress 800KB -f png` | PNG 无损，跳过（提示不支持有损压缩） |
| URL 作为 `-i` 输入（纯本地模式） | 跳过 URL，提示只接受本地文件 |
| 用 quality=5 仍超 target | 用 quality=5 输出，警告未达标 |

## 已知问题

### `grok-imagine` 模型 `--output-format base64` 返回异常

APIMart 上的 grok 模型在 `--output-format base64` 时会返回两个 URL：

```
URL[0]: "data:image/png;base64"        ← 空的数据 URI 前缀（无实际数据）
URL[1]: "/9j/4QhoRXhpZgA..."           ← 实际的图片 base64 数据
```

CLI 目前会把这个情况当成两个独立链接处理，可能都下载失败。如果遇到这种情况：

```bash
# 1. 找到目录下生成的 .txt 文件
ls -la *.txt

# 2. 第二个 .txt 就是完整的图片 base64，直接解码
base64 -d image_task_xxx_0_1.txt > output.png
```

推荐使用默认的 `--output-format url` 避免此问题。
