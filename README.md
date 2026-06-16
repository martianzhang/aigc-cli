# apimart-cli

APIMart API 的统一命令行工具。支持 **图片生成**、**视频生成** 和 **AI 对话**。

## 安装

```bash
go install github.com/martianzhang/apimart-cli@latest
```

或从源码构建：

```bash
git clone https://github.com/martianzhang/apimart-cli.git
cd apimart-cli
make build
```

## 配置

### API Key

三种设置方式（优先级从高到低）：

```bash
# 方式一：命令行参数
apimart-cli image --prompt "..." --api-key "sk-xxx"

# 方式二：环境变量
export APIMART_API_KEY="sk-xxx"

# 方式三：配置文件
```

### 配置文件

`~/.config/apimart/config.yaml` 可设置通用参数和默认值：

```yaml
api_key: "sk-xxx"

# API 地址（默认 https://api.apimart.ai）
# base_url: "https://api.apimart.ai"

# HTTP 代理（支持 http/https/socks5）
# 也可通过 APIMART_HTTP_PROXY 环境变量或 --http-proxy 参数设置
http_proxy: "http://127.0.0.1:7890"

defaults:
  image:
    model: "gpt-image-2-official"
    size: "3:1"
    resolution: "1k"
    quality: "low"
    output_format: "png"

  video:
    model: "grok-imagine-1.5-video-apimart" # $0.007，最便宜
    # grok 仅支持 model+prompt，其他参数注释参考
    # size: "16:9"
    # resolution: "480p"
    # duration: 5
```

### 提示词文件

每次生成任务完成后，会自动保存提示词到 `apimart_{task_id}.md` 文件，方便追溯：

```bash
# 默认保存
apimart-cli image --prompt "A red fox"

# 不保存
apimart-cli image --prompt "A red fox" --no-save-prompt
```

完整示例见 [config.example.yaml](config.example.yaml)。

## 命令结构

```
apimart-cli
├── image             图片生成（文生图、图生图、Inpainting）
├── video             视频生成（文生视频、图生视频、首尾帧）
├── chat              AI 对话（流式输出，默认 deepseek-v4-flash）
├── models            查询模型列表及定价（无需 API Key）
├── task               查询任务状态
├── balance            查询 Token 余额
└── balance user       查询账号余额
```

## 图片生成

支持文生图、图生图、Inpainting 三种模式。

### 基本用法（文生图）

```bash
# 直接传提示词
apimart-cli image --prompt "一只猫在星空下"

# 从文件读取（自动识别文件路径）
apimart-cli image --prompt prompt.txt

# 从 stdin 读取（--prompt 不传时默认读 stdin）
echo "赛博朋克城市夜景" | apimart-cli image
apimart-cli image < prompt.txt

# 自动轮询并下载图片到当前目录
apimart-cli image --prompt "..."
```

### 详细参数

所有 GPT-Image-2 参数均支持：

| 参数 | 说明 |
|---|---|
| `--prompt` | 文本描述（自动识别文件/stdin） |
| `--model` | 模型名，默认 `gpt-image-2-official` |
| `--size` | 宽高比，如 `16:9`、`1:1`，或像素如 `1024x1024` |
| `--resolution` | 分辨率档：`1k`、`2k`、`4k` |
| `--quality` | 质量：`auto`、`low`、`medium`、`high` |
| `--background` | 背景：`auto`、`opaque`、`transparent` |
| `--moderation` | 审核强度：`auto`、`low` |
| `--output-format` | 输出格式：`png`、`jpeg`、`webp` |
| `--output-compression` | 压缩率 0-100（jpeg/webp） |
| `--n` | 生成数量 1-4 |
| `--image-url` | 参考图片 URL（可重复） |
| `--mask-url` | 蒙版图片 URL（inpainting） |

```bash
apimart-cli image --prompt "..." \
  --size "16:9" \
  --resolution "2k" \
  --quality "high" \
  --output-format "jpeg" \
  --output-compression 90 \
  --n 2
```

### JSON 输入

构建完整的请求 JSON 传入：

```bash
# JSON 文件
apimart-cli image --json request.json

# JSON 字符串
apimart-cli image --json '{"prompt":"a red fox","n":4}'

# 从 stdin
cat request.json | apimart-cli image --json -
```

### 参考图生图 (image-to-image)

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

### Inpainting（蒙版替换）

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

> `--image-url` 和 `--mask-url` 可接受本地文件路径或远程 URL。
> 本地文件会自动通过 `POST /v1/uploads/images` 上传，上传后的 URL 有效期 72 小时。

### 最经济配置

参考 [APIMart 定价](https://apimart.ai/pricing)，`gpt-image-2-official` 最低 **$0.00144/张**：

```bash
apimart-cli image --prompt "..." \
  --size "3:1" \
  --resolution "1k" \
  --quality "low"
```

或写入 config.yaml 作为默认值。

## 代理

```bash
# 命令行指定（图片和视频通用）
apimart-cli image --prompt "..." --http-proxy "http://127.0.0.1:7890"
apimart-cli video --prompt "..." --http-proxy "http://127.0.0.1:7890"

# 环境变量（支持 HTTP_PROXY / HTTPS_PROXY / ALL_PROXY / NO_PROXY）
export HTTP_PROXY="http://127.0.0.1:7890"

# SOCKS5
apimart-cli image --prompt "..." --http-proxy "socks5://127.0.0.1:1080"
```

## 视频生成

支持文生视频、图生视频、首尾帧、参考视频、音频视频等模式。

```bash
# 文生视频（--prompt 不传时默认读 stdin）
apimart-cli video --prompt "A kitten yawning at the camera"
echo "A kitten yawning" | apimart-cli video

# 指定分辨率及时长
apimart-cli video --prompt "City nightscape" --resolution 720p --duration 8

# 图生视频（首帧）
apimart-cli video --prompt "The kitten walks toward the camera" --image-url ./cat.jpg

# 首尾帧过渡
apimart-cli video --prompt "Transition from day to night" \
  --first-frame day.jpg --last-frame night.jpg

# 生成带音频的视频
apimart-cli video --prompt "A man speaks to the camera" --generate-audio

# 参考视频 + 参考音频
apimart-cli video --prompt "A person speaking" \
  --video-url ./reference.mp4 --audio-url ./speech.wav

# JSON 输入
apimart-cli video --json request.json
```

### 视频参数

| 参数 | 说明 |
|---|---|
| `--prompt` | 视频内容描述 |
| `--model` | 模型名，默认 `grok-imagine-1.5-video-apimart`（$0.007）或 `doubao-seedance-2.0` |
| `--duration` | 时长 4-15 秒，默认 5 |
| `--size` | 宽高比：`16:9`、`9:16`、`1:1`、`4:3`、`3:4`、`21:9`、`adaptive` |
| `--resolution` | 分辨率：`480p`、`720p`、`1080p`，默认 `480p` |
| `--seed` | 随机种子，用于复现 |
| `--generate-audio` | 生成 AI 音频 |
| `--return-last-frame` | 返回最后一帧用于续拍 |
| `--image-url` | 参考图片 URL（可重复） |
| `--first-frame` | 首帧图片 |
| `--last-frame` | 尾帧图片 |
| `--video-url` | 参考视频 URL（可重复） |
| `--audio-url` | 参考音频 URL（可重复） |
| `--tool` | 工具（如 `web_search`，可重复） |

## AI 对话

支持流式输出（默认），兼容 OpenAI 格式，可使用 GPT-5、Claude、Gemini、DeepSeek 等模型。

```bash
# 基本对话（流式输出）
apimart-cli chat --message "你好，请介绍一下自己"

# 系统提示词
apimart-cli chat --system "你是一位诗人" --message "写一首关于AI的诗"

# 多轮对话
apimart-cli chat \
  --message "什么是机器学习？" \
  --message "能举个例子吗？"

# 非流式输出
echo "Explain Go in 3 words" | apimart-cli chat --no-stream

# 指定模型
apimart-cli chat --model gpt-5 --message "Hello"
```

### 对话参数

| 参数 | 说明 |
|---|---|
| `--message` | 用户消息（可重复，实现多轮对话） |
| `--system` | 系统提示词，设定 AI 角色 |
| `--model` | 模型名，默认 `deepseek-v4-flash`（最便宜） |
| `--temperature` | 采样温度 0-2，默认 1.0 |
| `--max-tokens` | 最大生成 token 数 |
| `--no-stream` | 关闭流式输出，等待完整响应 |
| `--json` | JSON 输入 |

## 其他命令

### 查询模型列表

无需 API Key，免认证查询市场可用模型：

```bash
# 所有模型
apimart-cli models

# 按类型筛选
apimart-cli models image
apimart-cli models video
apimart-cli models chat

# 或使用 --type 参数
apimart-cli models --type image

# 查看特定模型的详细定价
apimart-cli models pricing gpt-image-2-official
apimart-cli models pricing doubao-seedance-2.0
```

### 查询任务状态

```bash
apimart-cli task task_01KV4KD9FBH3AZ4DE18A7Y17S3
```

返回完整的任务信息（状态、进度、耗时、费用、结果 URL 等）。图片任务完成后自动下载图片到 `--output` 目录。

### 查询余额

```bash
# 查询当前 API Key（Token）的余额
apimart-cli balance

# 查询用户账号的总余额
apimart-cli balance user
```

### Dry-run 调试

打印即将提交的 curl 命令，不实际调用 API：

```bash
# 图片 dry-run
apimart-cli image --prompt "test" --size "16:9" --dry-run

# 视频 dry-run
apimart-cli video --prompt "test" --duration 4 --dry-run
```

## API 参考

| 端点 | 用途 | 状态 |
|---|---|---|
| `POST /v1/chat/completions` | AI 对话 | ✅ 已支持 |
| `POST /v1/images/generations` | 文生图 | ✅ 已支持 |
| `POST /v1/videos/generations` | 文生视频 | ✅ 已支持 |
| `POST /v1/uploads/images` | 上传图片 | ✅ 已支持 |
| `GET /v1/tasks/{task_id}` | 查询任务状态 | ✅ 已支持 |
| `GET /v1/balance` | Token 余额查询 | ✅ 已支持 |
| `GET /v1/user/balance` | 用户余额查询 | ✅ 已支持 |
| `GET /api/marketplace/models` | 模型列表（免认证） | ✅ 已支持 |
| `GET /api/pricing/model` | 模型定价详情（免认证） | ✅ 已支持 |

完整文档见 [docs.apimart.ai](https://docs.apimart.ai/en)。

## Makefile

```bash
make          # 编译
make run ARGS="image --help"   # 运行查看帮助
make clean    # 清理产物
make lint     # 静态检查
```

## 优先级规则

**CLI 参数 > JSON 输入 > YAML 配置 > 代码默认值**

代理优先级：
**`--http-proxy` 参数 > `APIMART_HTTP_PROXY` 环境变量 > `HTTP_PROXY` 标准环境变量**
