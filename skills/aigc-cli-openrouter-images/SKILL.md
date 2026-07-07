---
name: aigc-cli-openrouter-images
description: Use "aigc-cli image" with OpenRouter to generate images via the Responses API. Supports text-to-image with aspect ratio control. Automatically detects OpenRouter from --api-base / OPENAI_BASE_URL and uses POST /v1/responses with image output modalities.
---

# aigc-cli-openrouter-images

通过 `aigc-cli image` 调用 OpenRouter 的 Responses API（`POST /v1/responses`）生成图片。支持 `google/gemini-3.1-flash-image-preview` 等原生图片输出模型。

**自动兼容**：当 `--api-base` 或 `OPENAI_BASE_URL` 指向 `openrouter.ai` 时，工具自动使用 Responses API（而不是标准 OpenAI `/v1/images/generations` 端点）。

## 前置条件

1. 项目已安装 `aigc-cli`（`go install` 或 `make build`）
2. 已配置 OpenRouter API Key：
   ```bash
   export OPENAI_API_KEY="sk-or-xxx"
   export OPENAI_BASE_URL="https://openrouter.ai/api/v1"
   ```
3. 或在 `~/.config/aigc-cli/config.yaml` 中配置：
   ```yaml
   api_key: "sk-or-xxx"
   base_url: "https://openrouter.ai/api/v1"
   ```

## 何时使用

- 用户需要根据文本描述生成图片（走 OpenRouter）
- 用户使用了 `google/gemini-3.1-flash-image-preview` 等原生图片输出模型
- 用户已经配置了 OpenRouter API Key
- 用户需要指定宽高比（aspect ratio）

## 使用流程

### 1. 基本文生图

```bash
# 设置 OpenRouter
export OPENAI_API_KEY="sk-or-xxx"
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"

# 生成图片（自动识别 OpenRouter，使用 Responses API）
aigc-cli image --prompt "A red panda wearing sunglasses" \
  --model "google/gemini-3.1-flash-image-preview"
```

### 2. 指定宽高比

```bash
aigc-cli image --prompt "Futuristic cityscape at night" \
  --model "google/gemini-3.1-flash-image-preview" \
  --size "16:9"
```

支持 `--size`（宽高比，如 `16:9`、`1:1`、`4:3`）和 `--resolution`（如 `1k`、`2k`）。

### 3. 使用 OpenRouter 的 DALL-E 模型

虽然 DALL-E 模型走标准 `/v1/images/generations` 端点，但 OpenRouter 也支持：

```bash
# DALL-E 3（标准 OpenAI 兼容路径）
aigc-cli image --prompt "a cat" \
  --model "openai/dall-e-3" \
  --size "1024x1024"
```

当模型是 `openai/dall-e-*` 时，可以强制走同步模式：

```bash
aigc-cli image --prompt "a cat" \
  --model "openai/dall-e-3" \
  --mode sync
```

> 注意：Responses API 和标准 generations 端点会自动路由。OpenRouter 域名下默认使用 Responses API。

### 4. 完整参数

```bash
aigc-cli image \
  --prompt "提示词" \
  --model "google/gemini-3.1-flash-image-preview" \
  --size "16:9" \
  --resolution "2k"
```

### 5. JSON 输入

```bash
aigc-cli image --json '{
  "model": "google/gemini-3.1-flash-image-preview",
  "prompt": "your prompt",
  "size": "16:9",
  "resolution": "2k"
}'
```

## 常用 OpenRouter 图片模型

| 模型 ID | 说明 |
|---|---|
| `google/gemini-3.1-flash-image-preview` | Gemini 原生图片输出（默认推荐） |
| `google/gemini-2.5-flash-image` | Gemini 图片输出（旧版） |
| `openai/dall-e-3` | DALL-E 3（走同步模式） |
| `openai/dall-e-2` | DALL-E 2（走同步模式） |

使用 `aigc-cli models` 查看完整模型列表。

## 调试技巧

```bash
# 查看请求详情
aigc-cli image --prompt "test" -v

# Dry-run 查看 curl
aigc-cli image --prompt "test" --dry-run

# 指定输出目录
aigc-cli image --prompt "test" --output ./downloads
```

## 注意事项

- Responses API 返回 base64 编码的图片，自动解码保存到本地
- 响应中可能包含模型返回的文本描述（显示在控制台）
- 部分模型可能不支持 `--size`/`--resolution` 参数
- 首次使用建议 `--dry-run` 确认请求参数
- 不要多次调用 API 重复测试，避免产生不必要的费用
