# apimart-cli

[![CI](https://github.com/martianzhang/apimart-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/martianzhang/apimart-cli/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/martianzhang/apimart-cli)](https://go.dev/)
[![License](https://img.shields.io/github/license/martianzhang/apimart-cli)](LICENSE)
[![Release](https://img.shields.io/github/v/release/martianzhang/apimart-cli)](https://github.com/martianzhang/apimart-cli/releases)

> ⚠️ **非官方声明**：这是一个**个人开源项目**，与 APIMart 官方无关。代码基于 [APIMart](https://apimart.ai) 和 [OpenAI](https://openai.com) 公开 API 开发，使用前请确保遵守各平台的服务条款。

**一个 CLI，通吃 OpenAI、OpenRouter、APIMart 及任意 OpenAI 兼容中转服务。**

不只是 API 转发——智能适配各平台专有 API，完整覆盖图片/视频/Midjourney/AI 对话/提示词灵感，自带 MCP Server 和 Agentic Chat。

---

## ✨ 亮点

| | 能力 | 说明 |
|---|---|---|
| 🔌 | **多 Provider 统一入口** | 改一个 `base_url` 就能在 OpenAI / OpenRouter / APIMart / 任意中转之间切换，命令不变 |
| 🧠 | **Provider 自动适配** | 检测到 OpenRouter 自动走专用图片/视频 API，检测到 APIMart 走异步 Task，零配置 |
| 🎨 | **Midjourney 完整管线** | 17 个子命令覆盖 imagine → blend → describe → upscale → zoom → inpaint → video → remix，无需 Discord |
| 💬 | **Agentic Chat** | 交互式 REPL 内嵌 `generate_image` / `generate_video` / `midjourney_*` / `ideas` 等工具，LLM 直接在对话中生成图片视频 |
| 🤖 | **MCP Server** | 内置 MCP 协议支持，Claude Desktop / Cursor 等 AI 代理开箱即用 |
| 🔍 | **提示词灵感库** | 离线 BM25 搜索引擎（CJK 感知 + n-gram + RRF），千级提示词数据集，支持关键词 / 随机 / 图文搜索 |
| 🔄 | **视频任务持久化** | OpenRouter 视频提交→轮询→下载全流程，超时后 `--job-id` 一键恢复 |
| 🧪 | **Dry-Run & Curl** | `--dry-run` 输出等价 curl 命令，学习和调试 API 零门槛 |
| ⚡ | **Go 单二进制** | `go install` 一键安装，无 runtime 依赖，跨平台 |

---

## 快速开始

```bash
# 安装
go install github.com/martianzhang/apimart-cli@latest

# ── 使用 OpenAI ──
export OPENAI_API_KEY="sk-xxx"

apimart-cli image --prompt "一只猫在星空下"
apimart-cli chat --message "你是谁？"

# ── 使用 OpenRouter（改个环境变量，命令不用动）──
export OPENAI_API_KEY="sk-or-xxx"
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"

apimart-cli image --model "openai/gpt-image-2" --prompt "a cat"
apimart-cli video --model "google/veo-3.1" --prompt "a dog running"     # 自动走专用视频 API
apimart-cli models --type image                                           # 免认证模型发现

# ── 使用任意 OpenAI 兼容中转 ──
export OPENAI_API_KEY="sk-xxx"
export OPENAI_BASE_URL="https://your-relay.com/v1"

apimart-cli chat --message "Hello"
```

---

## Provider 自动适配

同样的 `image` / `video` / `models` 命令，背后走的 API 路径根据 Provider 自动切换：

| Provider | Image | Video | Models |
|---|---|---|---|
| **OpenAI** | `POST /v1/images/generations`（同步） | — | `GET /v1/models` |
| **OpenRouter** | `POST /v1/images`（专用图片 API）或 `POST /v1/responses`（Responses API） | `POST /v1/videos` 异步→轮询→下载 + `--job-id` 恢复 | `GET /v1/images/models` / `GET /v1/videos/models`（免认证） |
| **APIMart** | 异步 Task 提交→轮询→下载 | 异步 Task + VEO3 Remix（延长视频） | 市场 API + 模型定价查询 |
| **云雾 AI** | — | `POST /v1/video/create` + `GET /v1/video/query` | — |
| **通用中转** | `POST /v1/images/generations`（同步） | — | `GET /v1/models` |

检测逻辑：根据 `base_url` 自动识别，也可用 `--mode sync` / `--mode async` 手动指定。

---

## 命令

```
apimart-cli
├── image      图片生成（同步/异步/OpenRouter 专用 API / Grok Edit）        →  docs/guide-image.md
├── video      视频生成（APIMart / OpenRouter / 云雾 + VEO3 Remix）          →  docs/guide-video.md
├── midjourney Midjourney 完整流水线（17 子命令）                          →  docs/guide-midjourney.md
│   └── mj     别名，同上
├── chat       AI 对话 / 交互式 REPL / Agent Loop（工具调用）              →  docs/guide-chat.md
├── ideas      提示词灵感搜索（关键词 / 随机 / 图文，支持 BM25 + n-gram）   →  docs/guide-ideas.md
├── models     模型列表（APIMart 市场 / OpenRouter 发现 / OpenAI 兼容）
│   └── --price    查看模型定价
├── task       查询异步任务状态（APIMart）
├── balance    查询余额（APIMart）
├── preview    用系统默认程序预览生成的图片/视频
├── mcp        启动 MCP Server（AI 代理集成）                              →  docs/mcp.md
│
│   # 全局标志
│   --dry-run      打印请求参数和等价 curl，不调用 API
│   --print-config 打印当前生效的配置（含来源标注）
│   -v/--verbose   显示详细输出：完整 JSON、Token 用量、耗时、费用
│   --json         以 JSON 格式传入请求（文件、字符串或 stdin）
│   --preview      生成完成后自动打开系统预览
│   --save-prompt  将提示词保存为 .md 文件
│   --http-proxy   指定 HTTP 代理
```

### Midjourney 子命令一览

```
apimart-cli midjourney (或 mj)
├── imagine       文生图 / 图生图（默认入口）
├── blend         多图融合（2-4 张）
├── describe      图转文（反向提示词）
├── edits         图片编辑（重写整图）
├── upscale       放大单张（U1-U4）
├── variation     微变体（V1-V4）
├── high-variation 强变体
├── low-variation  弱变体
├── reroll        重新生成网格
├── zoom          拉远 / 外绘
├── pan           平移（左/右/上/下）
├── inpaint       局部重绘入口（→ modal）
├── modal         提交蒙版 + 提示词完成重绘
├── video         图生视频
├── remix-strong  强重塑（v8/v8.1）
├── remix-subtle  弱重塑（v8/v8.1）
└── query         查询任务状态
```

---

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

---

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

---

## 优先级规则

**CLI 参数 > JSON 输入 > YAML 配置 > 代码默认值**

代理优先级：
**`--http-proxy` 参数 > `OPENAI_HTTP_PROXY` / `APIMART_HTTP_PROXY` 环境变量 > `HTTP_PROXY` 标准环境变量**

---

## 贡献

欢迎贡献！详见 [CONTRIBUTING.md](CONTRIBUTING.md)。

---

## License

[MIT](LICENSE)

<div align="center">

<img src="wechat.jpg" width="400" alt="没有那多" />

**欢迎关注微信公众号"没有那多"，分享更多好用、好用的工具**

</div>
