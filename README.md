# apimart-cli

[![CI](https://github.com/martianzhang/apimart-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/martianzhang/apimart-cli/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/martianzhang/apimart-cli)](https://go.dev/)
[![License](https://img.shields.io/github/license/martianzhang/apimart-cli)](LICENSE)
[![Release](https://img.shields.io/github/v/release/martianzhang/apimart-cli)](https://github.com/martianzhang/apimart-cli/releases)

> ⚠️ **非官方声明**：这是一个**个人开源项目**，与 APIMart 官方无关。代码基于 [APIMart](https://apimart.ai) 和 [OpenAI](https://openai.com) 公开 API 开发，使用前请确保遵守各平台的服务条款。

OpenAI 兼容 API 的统一命令行工具。支持 **图片生成**、**视频生成** 和 **AI 对话**。

兼容 [OpenAI](https://openai.com)、[OpenRouter](https://openrouter.ai)、[APIMart](https://apimart.ai) 及任意 OpenAI 兼容的第三方中转服务。

## 快速开始

```bash
# 安装
go install github.com/martianzhang/apimart-cli@latest

# 设置 API Key（支持 OPENAI_API_KEY 或 APIMART_API_KEY）
export OPENAI_API_KEY="sk-xxx"

# 生成一张图
apimart-cli image --prompt "一只猫在星空下"

# AI 对话
apimart-cli chat --message "你好"

# 看更多用法
apimart-cli image --help
```

### 使用 OpenRouter

当 `base_url` 指向 `openrouter.ai` 时，工具自动适配 OpenRouter 的专有 API：

| 命令 | OpenRouter 自动适配 |
|---|---|
| `image` | `POST /v1/images`（专用图片 API）或 `POST /v1/responses`（Responses API） |
| `video` | `POST /v1/videos`（异步提交 → 轮询 → 下载） |
| `models --type image\|video` | `GET /v1/images/models` / `GET /v1/videos/models`（免认证模型发现） |

```bash
export OPENAI_API_KEY="sk-or-xxx"
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"

# 自动走 OpenRouter 专用图片 API
apimart-cli image --model "openai/gpt-image-2" --prompt "a cat"

# 自动走 OpenRouter 视频 API（提交 → 轮询 → 下载）
apimart-cli video --model "google/veo-3.1" --prompt "a dog running"

# 视频任务持久化，超时后可重新拉取
apimart-cli video --job-id vid_xxx

# 查看 OpenRouter 图片/视频模型（免认证）
apimart-cli models --type image
apimart-cli models --type video

# AI 对话
apimart-cli chat --model "openai/gpt-4.1-nano" --message "Hello"
```

### 使用任意 OpenAI 兼容中转

```bash
export OPENAI_API_KEY="sk-xxx"
export OPENAI_BASE_URL="https://your-relay.com/v1"

apimart-cli chat --message "Hello"
```

## 命令

```
apimart-cli
├── image      图片生成（同步/异步/OpenRouter 专用 API）→  docs/guide-image.md
├── video      视频生成（APIMart 异步 / OpenRouter 异步）→  docs/guide-video.md
├── midjourney Midjourney 生成/编辑    →  docs/guide-midjourney.md
│   └── mj     别名，同上
├── chat       AI 对话               →  docs/guide-chat.md
├── ideas      提示词灵感搜索          →  docs/guide-ideas.md
├── models     模型列表（APIMart 市场 / OpenRouter 发现 / OpenAI 兼容）
├── task       查询任务状态（APIMart）
├── balance    查询余额（APIMart）
└── mcp        MCP Server 🧪        →  docs/mcp.md
```

## 文档

| 文档 | 内容 |
|---|---|
| [安装与配置](docs/installation.md) | 安装、API Key、配置文件、代理 |
| [图片生成](docs/guide-image.md) | 全部参数、同步/异步模式、图生图、Inpainting |
| [视频生成](docs/guide-video.md) | 全部参数、首尾帧、参考视频（APIMart） |
| [Midjourney 生成](docs/guide-midjourney.md) | 17 个子命令完整说明：imagine、blend、upscale 等 |
| [AI 对话](docs/guide-chat.md) | 交互式多轮 REPL、流式输出、verbose 统计 |
| [提示词灵感](docs/guide-ideas.md) | 从 Image2Studio 搜索 AI 图片提示词灵感 |
| [其他命令](docs/guide-commands.md) | models、task、balance、dry-run、API 参考 |
| [API 参考来源](docs/api-reference.md) | 各 Provider 接口规范来源、检测机制、策略路由 |
| [常见问题](docs/faq.md) | 安装、使用、MCP、费用等常见问题解答 |
| [MCP 集成](docs/mcp.md) | AI 代理（Claude/Cursor）集成指南 |

## MCP 集成 🧪（测试中）

```json
{
  "mcpServers": {
    "apimart": {
      "command": "apimart-cli",
      "args": ["mcp"]
    }
  }
}
```

详见 [docs/mcp.md](docs/mcp.md)。

## 优先级规则

**CLI 参数 > JSON 输入 > YAML 配置 > 代码默认值**

代理优先级：
**`--http-proxy` 参数 > `OPENAI_HTTP_PROXY` / `APIMART_HTTP_PROXY` 环境变量 > `HTTP_PROXY` 标准环境变量**

## 贡献

欢迎贡献！详见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## License

[MIT](LICENSE)

<div align="center">

<img src="wechat.jpg" width="400" alt="没有那多" />

**欢迎关注微信公众号"没有那多"，分享更多好用、好用的工具**

</div>
