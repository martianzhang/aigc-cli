# 其他命令

## 检测水印、元数据和 AIGC 信号

综合分析图片中的多种信号，输出 AI 生成置信度（AIGen rate）+ emoji。

**支持信号：**
| 信号 | 权重 | 说明 |
|---|---|---|
| C2PA Content Credentials | 🔴 铁证 | Adobe/OpenAI 等生态，签名验证 |
| TC260 AIGC Label | 🔴 铁证 | 中国 GB 45438-2025 国标 |
| SynthID 水印推断 | 🟠 高 | Google/OpenAI 厂商匹配 |
| Camera EXIF | 🟢 强人类 | 有相机信息=实拍 |
| ONNX 模型 (ViT-Base) | 🟡 中 | 86M 参数 ML 模型 |
| JPEG 量化表分析 | 🟡 中 | 检测非标准量化表 |
| SRM 噪声残差 | 🔵 低 | 5×5 高通滤波分析 |
| FFT 频谱分析 | 🔵 低 | 频域功率谱偏差 |
| 无 EXIF | 🔵 弱 | 截图/AI 图常见 |

```bash
# 基础检测（自动融合所有信号）
apimart-cli detect image.png
# → 🤖 99% Confirmed AI-generated (TC260)
# → 🟡 35% Slightly suspicious (No EXIF + ONNX + FFT)

# JSON 输出（用于脚本处理）
apimart-cli detect --json image.png

# 检测并打开系统看图软件
apimart-cli detect --preview image.png

# 检测多张图片
apimart-cli detect *.png

# 从管道读取
cat image.png | apimart-cli detect
```

### ONNX 模型检测（需下载模型）

```bash
# 下载大模型（ViT-Base 86M，推荐）
apimart-cli detect init

# 下载小模型（distilled ViT 11.8M）
apimart-cli detect init --size small

# 强制重新下载
apimart-cli detect init --force

# 下载后自动启用，输出示例：
apimart-cli detect image.png
# AI Detect:  🟠 58%  Possibly AI-generated
#   No Camera EXIF=55%; AI Model=73%; FFT Spectral=6%
```

**特点：**
- 完全离线运行，无需 API Key
- 支持 PNG、JPEG、WebP、GIF、BMP
- 多信号加权融合，emoji 一览
- ONNX 模型需先运行 `detect init` 下载
- SynthID 检测基于 C2PA 元数据推断
- TC260 标签可识别国内主要厂商
- JPEG 自动提取相机 EXIF 和量化表分析
- FFT 频谱分析检测 GAN/扩散模型痕迹

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
