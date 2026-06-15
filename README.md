# apimart-cli

APIMart API 的统一命令行工具。当前支持 **文生图（text-to-image）**，后续将扩展视频生成等更多能力。

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
apimart-cli generate --prompt "..." --api-key "sk-xxx"

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
  model: "gpt-image-2-official"
  size: "3:1"
  resolution: "1k"
  quality: "low"
  output_format: "png"
```

完整示例见 [config.example.yaml](config.example.yaml)。

## 图片生成

支持文生图、图生图、Inpainting 三种模式。

### 基本用法（文生图）

```bash
# 直接传提示词
apimart-cli generate --prompt "一只猫在星空下"

# 从文件读取（自动识别文件路径）
apimart-cli generate --prompt prompt.txt

# 从 stdin 读取
echo "赛博朋克城市夜景" | apimart-cli generate --prompt -

# 自动轮询并下载图片到当前目录
apimart-cli generate --prompt "..."
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
apimart-cli generate --prompt "..." \
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
apimart-cli generate --json request.json

# JSON 字符串
apimart-cli generate --json '{"prompt":"a red fox","n":4}'

# 从 stdin
cat request.json | apimart-cli generate --json -
```

### 参考图生图 (image-to-image)

参考已有图片进行融合或编辑，支持本地文件（自动上传）和远程 URL：

```bash
# 本地文件（自动上传到 APIMart）
apimart-cli generate \
  --prompt "把这张照片改成吉卜力风格" \
  --image-url ./my-photo.jpg

# 远程 URL
apimart-cli generate \
  --prompt "融合两张参考图，保留主要轮廓" \
  --image-url "https://example.com/img1.png" \
  --image-url "https://example.com/img2.png"
```

### Inpainting（蒙版替换）

提供原图和蒙版，替换指定区域：

```bash
# 本地文件自动上传
apimart-cli generate \
  --prompt "把背景换成沙漠日落" \
  --image-url ./photo.png \
  --mask-url ./mask.png

# 远程 URL
apimart-cli generate \
  --prompt "Replace background with desert sunset" \
  --image-url "https://example.com/photo.png" \
  --mask-url "https://example.com/mask.png"
```

> `--image-url` 和 `--mask-url` 可接受本地文件路径或远程 URL。
> 本地文件会自动通过 `POST /v1/uploads/images` 上传，上传后的 URL 有效期 72 小时。

### 最经济配置

参考 [APIMart 定价](https://apimart.ai/pricing)，`gpt-image-2-official` 最低 **$0.00144/张**：

```bash
apimart-cli generate --prompt "..." \
  --size "3:1" \
  --resolution "1k" \
  --quality "low"
```

或写入 config.yaml 作为默认值。

## 代理

```bash
# 命令行指定
apimart-cli generate --prompt "..." --http-proxy "http://127.0.0.1:7890"

# 环境变量（支持 HTTP_PROXY / HTTPS_PROXY / ALL_PROXY / NO_PROXY）
export HTTP_PROXY="http://127.0.0.1:7890"

# SOCKS5
apimart-cli generate --prompt "..." --http-proxy "socks5://127.0.0.1:1080"
```

## 其他命令

### 查询任务状态

```bash
apimart-cli task task_01KV4KD9FBH3AZ4DE18A7Y17S3
```

返回完整的任务信息（状态、进度、耗时、费用、结果图片 URL 等）。任务完成后会自动下载图片到 `--output` 目录。

### 查询余额

```bash
# 查询当前 API Key（Token）的余额
apimart-cli balance

# 查询用户账号的总余额
apimart-cli balance user
```

## API 参考

| 端点 | 用途 | 状态 |
|---|---|---|
| `POST /v1/images/generations` | 文生图 | ✅ 已支持 |
| `GET /v1/tasks/{task_id}` | 查询任务状态 | ✅ 已支持 |
| `GET /v1/balance` | Token 余额查询 | ✅ 已支持 |
| `GET /v1/user/balance` | 用户余额查询 | ✅ 已支持 |
| `POST /v1/uploads/images` | 上传图片 | ✅ 已支持 |
| `POST /v1/videos/generations` | 文生视频 | 🚧 待开发 |

完整文档见 [docs.apimart.ai](https://docs.apimart.ai/en)。

## Makefile

```bash
make          # 编译
make run ARGS="generate --help"   # 运行
make clean    # 清理产物
make lint     # 静态检查
```

## 优先级规则

**CLI 参数 > JSON 输入 > YAML 配置 > 代码默认值**

代理优先级：
**`--http-proxy` 参数 > `APIMART_HTTP_PROXY` 环境变量 > `HTTP_PROXY` 标准环境变量**
