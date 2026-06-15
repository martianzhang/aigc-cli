---
name: apimart-text2image
description: Use the apimart-cli tool to generate images via the APIMart GPT-Image-2 API. Supports text-to-image, image-to-image, inpainting, configurable resolution/quality, proxy, and task polling with --wait.
---

# apimart-text2image

通过 `apimart-cli` 调用 APIMart GPT-Image-2 API 生成图片。

## 前置条件

1. 项目已安装 `apimart-cli`（`go install` 或 `make build`）
2. 已配置 API Key（`~/.config/apimart/config.yaml` 或 `APIMART_API_KEY` 环境变量）

## 何时使用

- 用户需要根据文本描述生成图片
- 用户需要参考图片进行图生图或 inpainting
- 用户需要批量生成多张图片
- 用户需要指定分辨率、质量、宽高比等参数

## 使用流程

### 1. 基本文生图

```bash
apimart-cli generate --prompt "你的提示词" --wait --output .
```

`--wait` 会轮询任务直到完成并自动下载图片到当前目录。

### 2. 详细参数

```bash
apimart-cli generate \
  --prompt "提示词" \
  --model "gpt-image-2-official" \
  --size "16:9" \
  --resolution "2k" \
  --quality "high" \
  --output-format "jpeg" \
  --n 1 \
  --wait \
  --output ./output
```

### 3. 长提示词

提示词较长时，写入文件后传给 `--prompt`（自动识别文件）：

```bash
cat > prompt.txt << 'EOF'
详细的图片描述...
EOF
apimart-cli generate --prompt prompt.txt --wait
```

或通过 stdin：

```bash
echo "详细描述" | apimart-cli generate --prompt -
```

### 4. JSON 输入

```bash
apimart-cli generate --json '{
  "model": "gpt-image-2-official",
  "prompt": "your prompt",
  "size": "16:9",
  "resolution": "2k",
  "n": 2
}' --wait
```

### 5. 图生图 / Inpainting

```bash
apimart-cli generate \
  --prompt "融合两张参考图" \
  --image-url "https://example.com/img1.png" \
  --image-url "https://example.com/img2.png" \
  --wait
```

```bash
# Inpainting：替换背景
apimart-cli generate \
  --prompt "把背景换成沙漠日落" \
  --image-url "https://example.com/photo.png" \
  --mask-url "https://example.com/mask.png" \
  --wait
```

## 最经济配置

参考定价页 https://apimart.ai/pricing

`gpt-image-2-official` 最低 **$0.00144/张**：

```bash
apimart-cli generate \
  --prompt "提示词" \
  --size "3:1" \
  --resolution "1k" \
  --quality "low" \
  --wait
```

或设入 config.yaml 作为全局默认值。

## 代理

如果用户环境需要代理才能访问外网：

```bash
# 命令行
apimart-cli generate --prompt "..." --http-proxy "http://127.0.0.1:7890"

# 环境变量（自动识别）
export HTTP_PROXY="http://127.0.0.1:7890"

# 或 config.yaml
# http_proxy: "http://127.0.0.1:7890"
```

支持 `http://`、`https://`、`socks5://` 协议。

## 注意事项

- `--wait` 会轮询直到任务完成，最长等待 180 秒
- `quality: "high"` + `resolution: "2k"/"4k"` 可能耗时 120 秒以上
- 生成图片的 URL 有时效性，建议用 `--wait` 自动下载到本地
- 不要多次调用 API 测试，避免产生不必要的费用
