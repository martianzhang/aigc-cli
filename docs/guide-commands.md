# 其他命令

## 检测水印和元数据

检测图片中的 C2PA Content Credentials、TC260 AIGC 标签（中国 GB 45438-2025）和 SynthID 隐形水印。

```bash
# 显示所有信息（文件大小、尺寸、格式、水印、嵌入文本）
apimart-cli detect image.png

# JSON 输出（用于脚本处理）
apimart-cli detect --json image.png

# 检测多张图片
apimart-cli detect *.png

# 从管道读取
cat image.png | apimart-cli detect
```

**特点：**
- 完全离线运行，无需 API Key
- 支持 PNG、JPEG、WebP、GIF、BMP
- SynthID 检测基于 C2PA 元数据推断（Google Imagen/Gemini、OpenAI DALL-E 等）
- TC260 标签可识别国内主要厂商（字节跳动、百度、腾讯、阿里巴巴、智谱AI）
- JPEG 文件自动提取相机 EXIF 信息（厂商、型号、镜头、焦距、光圈、ISO、快门速度），可作为判断真实照片或 AI 生成图片的参考

## 查询模型列表

支持三个数据源，自动根据 API 地址选择：

| base_url | `--type` 行为 | `--price` 行为 | 无参数行为 |
|---|---|---|---|
| APIMart 域名 | `GET /api/marketplace/models?type=...` | APIMart 定价 API | `GET /v1/models` |
| OpenRouter 域名 | `GET /v1/images\|videos/models`（能力发现） | — | `GET /v1/models` |
| 其他（OpenAI 等） | `GET /v1/models` | — | `GET /v1/models` |

```bash
# 自动选择数据源
apimart-cli models

# APIMart 市场（按类型筛选）
apimart-cli models --type image
apimart-cli models --type video
apimart-cli models --type chat

# APIMart 特定模型定价
apimart-cli models --price gpt-image-2-official

# OpenRouter 模型发现（免认证，无需 API Key）
# 自动调用 /v1/images/models 或 /v1/videos/models
apimart-cli models --type image   # 展示架构、参数、能力
apimart-cli models --type video

# OpenAI 标准模型列表
apimart-cli models --base-url "https://api.openai.com/v1"
```

## 查询任务状态

仅 APIMart 异步模式可用：

```bash
apimart-cli task task_01KV4KD9FBH3AZ4DE18A7Y17S3
```

返回完整的任务信息（状态、进度、耗时、费用、结果 URL 等）。图片任务完成后自动下载图片到 `--output` 目录。

## 查询余额

仅 APIMart 可用：

```bash
# 查询当前 API Key（Token）的余额
apimart-cli balance

# 查询用户账号的总余额
apimart-cli balance user
```

## Dry-run 调试

打印即将提交的 curl 命令，不实际调用 API：

```bash
# 图片 dry-run
apimart-cli image --prompt "test" --size "16:9" --dry-run

# 视频 dry-run
apimart-cli video --prompt "test" --duration 4 --dry-run

# Midjourney dry-run
apimart-cli mj imagine --prompt "test" --dry-run
apimart-cli mj upscale --task-id task_xxx --index 1 --dry-run
```

## 查看版本

```bash
apimart-cli version
# 或
apimart-cli --version
```

## API 参考

> 各端口的接口规范详细参考来源见 [api-reference.md](api-reference.md)。

| 端点 | 用途 | 适用 | 参考来源 |
|---|---|---|---|
| `POST /v1/chat/completions` | AI 对话 | 通用 ✅ | [OpenAI Chat](https://platform.openai.com/docs/api-reference/chat/create) |
| `POST /v1/images/generations` | 文生图（同步/异步） | 通用 ✅ | [OpenAI Images](https://platform.openai.com/docs/api-reference/images/create) / [APIMart](https://docs.apimart.ai/en) |
| `POST /v1/images` | 文生图（OpenRouter 专用 API，支持 input_references） | OpenRouter ✅ | [OpenRouter Image](https://openrouter.ai/docs/guides/overview/multimodal/image-generation) |
| `POST /v1/responses` | 文生图（OpenRouter Responses API，原生图片输出模型） | OpenRouter ✅ | [OpenRouter Responses](https://openrouter.ai/docs/guides/overview/multimodal/image-generation) |
| `POST /v1/videos/generations` | 文生视频 | APIMart ✅ | [APIMart Docs](https://docs.apimart.ai/en) |
| `POST /v1/videos` | 文生视频（异步 submit → poll → download） | OpenRouter ✅ | [OpenRouter Video](https://openrouter.ai/docs/guides/overview/multimodal/video-generation) |
| `POST /v1/video/create` | 文生视频 | 云雾 Yunwu ✅ | 云雾 API 文档 |
| `GET /v1/images/models` | 图片模型发现（免认证，含参数能力描述） | OpenRouter ✅ | [OpenRouter Image Models](https://openrouter.ai/docs/api/api-reference/images/list-image-models) |
| `GET /v1/videos/models` | 视频模型发现（免认证） | OpenRouter ✅ | [OpenRouter Video Models](https://openrouter.ai/docs/api/api-reference/video-generation/list-videos-models) |
| `POST /v1/midjourney/generations` (及 16 个子端点) | Midjourney 图生/编辑 | APIMart ✅ | [APIMart Docs](https://docs.apimart.ai/en) |
| `POST /v1/uploads/images` | 上传图片 | APIMart ✅ | [APIMart Docs](https://docs.apimart.ai/en) |
| `GET /v1/tasks/{task_id}` | 查询任务状态 | APIMart ✅ | [APIMart Docs](https://docs.apimart.ai/en) |
| `GET /v1/midjourney/{task_id}` | 查询 MJ 任务（含 buttons） | APIMart ✅ | [APIMart Docs](https://docs.apimart.ai/en) |
| `GET /v1/balance` | Token 余额查询 | APIMart ✅ | [APIMart Docs](https://docs.apimart.ai/en) |
| `GET /v1/user/balance` | 用户余额查询 | APIMart ✅ | [APIMart Docs](https://docs.apimart.ai/en) |
| `GET /api/marketplace/models` | 模型列表（免认证） | APIMart ✅ | [APIMart Docs](https://docs.apimart.ai/en) |
| `GET /api/pricing/model` | 模型定价详情（免认证） | APIMart ✅ | [APIMart Docs](https://docs.apimart.ai/en) |
| `GET /api/image2studio.com/public/prompts/search` | 提示词灵感搜索 | 通用 ✅ | [Image2Studio](https://image2studio.com/prompts) |
| `GET /v1/models` | 模型列表 | OpenAI/OpenRouter ✅ | [OpenAI Models](https://platform.openai.com/docs/api-reference/models/list) / [OpenRouter Models](https://openrouter.ai/docs/api/api-reference/models/get-models) |

各端口的接口规范详细参考来源、Provider 检测机制和策略路由说明见 [api-reference.md](api-reference.md)。
