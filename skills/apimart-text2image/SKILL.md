---
name: apimart-text2image
description: Use "apimart-cli image" to generate images via OpenAI-compatible APIs (APIMart, OpenAI, OpenRouter). Supports text-to-image, image-to-image, inpainting, Grok Imagine Edit (--edit), local file upload, dry-run, proxy, sync/async mode detect. Automatically polls task and downloads images.
---

# apimart-text2image

通过 `apimart-cli image` 调用 OpenAI 兼容 API 生成图片。支持 APIMart（异步任务）、OpenAI / OpenRouter（同步），自动检测 API 地址并选择对应模式。也支持 Grok Imagine 1.5 Edit 图片编辑（`--edit` 模式）。提交任务后自动轮询完成并下载图片到当前目录。

## 前置条件

1. 项目已安装 `apimart-cli`（`go install` 或 `make build`）
2. 已配置 API Key（`~/.config/apimart/config.yaml` 或 `OPENAI_API_KEY` / `APIMART_API_KEY` 环境变量）
   - 图片默认参数在 `defaults.image` 下配置
   - Midjourney / Grok 等模型参数在 `defaults.midjourney` 下配置

## 何时使用

- 用户需要根据文本描述生成图片
- 用户需要参考图片进行图生图或 inpainting
- 用户需要批量生成多张图片
- 用户需要指定分辨率、质量、宽高比等参数

## 使用流程

### 1. 基本文生图

```bash
# 直接传提示词
apimart-cli image --prompt "你的提示词"

# --prompt 不传时默认读 stdin
echo "你的提示词" | apimart-cli image
apimart-cli image < prompt.txt
```

提交后自动轮询，任务完成即下载图片到当前目录。

### 2. 同步模式（OpenAI / OpenRouter）

自动检测 API 地址，APIMart 域名走异步，其他（openai.com、openrouter.ai 等）走同步：

```bash
# 自动同步模式
apimart-cli image --base-url "https://openrouter.ai/api/v1" \
  --prompt "a cat" --model "openai/dall-e-3"

# 强制指定模式
apimart-cli image --mode sync --prompt "..."
apimart-cli image --mode async --prompt "..."
```

同步模式支持 `--style vivid|natural` 和 `--response-format url|b64_json`。

### 3. 完整参数

```bash
apimart-cli image \
  --prompt "提示词" \
  --model "gpt-image-2-official" \
  --size "16:9" \
  --resolution "2k" \
  --quality "high" \
  --output-format "jpeg" \
  --n 1 \
  --output ./output \
  --style vivid \
  --background transparent \
  --output-compression 85
```

### 4. 长提示词

提示词较长时，写入文件后传给 `--prompt`（自动识别文件）：

```bash
cat > prompt.txt << 'EOF'
详细的图片描述...
EOF
apimart-cli image --prompt prompt.txt
```

或通过 stdin：

```bash
echo "详细描述" | apimart-cli image
```

### 5. JSON 输入

```bash
apimart-cli image --json '{
  "model": "gpt-image-2-official",
  "prompt": "your prompt",
  "size": "16:9",
  "resolution": "2k",
  "n": 2
}'
```

### 6. 图生图 / Inpainting

```bash
apimart-cli image \
  --prompt "融合两张参考图" \
  --image-url "https://example.com/img1.png" \
  --image-url "https://example.com/img2.png"
```

```bash
# Inpainting：替换背景
apimart-cli image \
  --prompt "把背景换成沙漠日落" \
  --image-url "https://example.com/photo.png" \
  --mask-url "https://example.com/mask.png"
```

### 7. Grok Imagine 1.5 Edit（图片编辑）

> ⚠️ 仅 Grok Imagine 1.5 Edit 模型支持，不是所有模型都有此功能。

```bash
# 背景替换
apimart-cli image --edit \
  --prompt "把背景换成星空，保留主体" \
  --image-url ./photo.jpg

# 风格迁移
apimart-cli image --edit \
  --prompt "换成赛博朋克风格" \
  --image-url ./img.png \
  --n 2
```

`--edit` 模式下自动默认模型 `grok-imagine-1.5-edit-apimart`，`--image-url` 必填。

### 8. 本地文件自动上传

`--image-url` 和 `--mask-url` 支持本地文件路径，自动上传：

```bash
apimart-cli image --prompt "吉卜力风格" --image-url ./my-photo.jpg
apimart-cli image --prompt "换背景" --image-url ./photo.png --mask-url ./mask.png
```

### 8. Dry-run 调试

查看即将提交的 curl 命令，不实际调用 API：

```bash
apimart-cli image --prompt "test" --size "16:9" --dry-run
```

## 最经济配置

参考定价页 https://apimart.ai/pricing

`gpt-image-2-official` 最低 **$0.00144/张**：

```bash
apimart-cli image \
  --prompt "提示词" \
  --size "3:1" \
  --resolution "1k" \
  --quality "low"
```

或设入 config.yaml 作为全局默认值。

## 代理

如果用户环境需要代理才能访问外网：

```bash
# --http-proxy 参数（支持 http/https/socks5）
apimart-cli image --prompt "..." --http-proxy "http://127.0.0.1:7890"

# 环境变量（自动识别）
export HTTP_PROXY="http://127.0.0.1:7890"

# 或 config.yaml
# http_proxy: "http://127.0.0.1:7890"
```

支持 `http://`、`https://`、`socks5://` 协议。

## 调试技巧

```bash
# 查看即将提交的完整请求 JSON
apimart-cli image --prompt "test" -v

# Dry-run：打印 curl 命令，不实际调用
apimart-cli image --prompt "test" --dry-run

# 保存 prompt 到 image_{task_id}.md
apimart-cli image --prompt "A red fox" --save-prompt
```

## 注意事项

- 提交后自动轮询任务，最长等待 180 秒
- `quality: "high"` + `resolution: "2k"/"4k"` 可能耗时 120 秒以上
- 图片自动下载到当前目录（或用 `--output` 指定目录），无需额外操作
- 不要多次调用 API 测试，避免产生不必要的费用
