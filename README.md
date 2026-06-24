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

```bash
export OPENAI_API_KEY="sk-or-xxx"
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"

apimart-cli chat --model "openai/gpt-4o" --message "Hello"
apimart-cli image --model "openai/dall-e-3" --prompt "a cat"
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
├── image      图片生成（同步/异步）→  docs/guide-image.md
├── video      视频生成               →  docs/guide-video.md
├── chat       AI 对话               →  docs/guide-chat.md
├── models     模型列表及定价
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
| [AI 对话](docs/guide-chat.md) | 交互式多轮 REPL、流式输出、verbose 统计 |
| [其他命令](docs/guide-commands.md) | models、task、balance、dry-run、API 参考 |
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

## License

[MIT](LICENSE)

<div align="center">

<img src="wechat.jpg" width="400" alt="没有那多" />

**欢迎关注微信公众号"没有那多"，分享更多好用、好用的工具**

</div>
