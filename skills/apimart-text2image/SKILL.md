---
name: apimart-text2image
description: Use "apimart-cli image" to generate images via the APIMart GPT-Image-2 API. Supports text-to-image, image-to-image, inpainting, local file upload, dry-run, proxy. Automatically polls task and downloads images.
---

# apimart-text2image

通过 `apimart-cli image` 调用 APIMart GPT-Image-2 API 生成图片。提交任务后自动轮询完成并下载图片到当前目录。

## 前置条件

1. 项目已安装 `apimart-cli`（`go install` 或 `make build`）
2. 已配置 API Key（`~/.config/apimart/config.yaml` 或 `APIMART_API_KEY` 环境变量）
   - 图片默认参数在 `defaults.image` 下配置

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

### 2. 详细参数

```bash
apimart-cli image \
  --prompt "提示词" \
  --model "gpt-image-2-official" \
  --size "16:9" \
  --resolution "2k" \
  --quality "high" \
  --output-format "jpeg" \
  --n 1 \
  --output ./output
```

### 3. 长提示词

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

### 4. JSON 输入

```bash
apimart-cli image --json '{
  "model": "gpt-image-2-official",
  "prompt": "your prompt",
  "size": "16:9",
  "resolution": "2k",
  "n": 2
}'
```

### 5. 图生图 / Inpainting

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

### 6. 本地文件自动上传

`--image-url` 和 `--mask-url` 支持本地文件路径，自动上传：

```bash
apimart-cli image --prompt "吉卜力风格" --image-url ./my-photo.jpg
apimart-cli image --prompt "换背景" --image-url ./photo.png --mask-url ./mask.png
```

### 7. Dry-run 调试

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

## 注意事项

- 提交后自动轮询任务，最长等待 180 秒
- `quality: "high"` + `resolution: "2k"/"4k"` 可能耗时 120 秒以上
- 图片自动下载到当前目录（或用 `--output` 指定目录），无需额外操作
- 不要多次调用 API 测试，避免产生不必要的费用
