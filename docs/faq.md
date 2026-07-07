# 常见问题

## 安装与配置

### 如何安装？

```bash
go install github.com/martianzhang/apimart-cli@latest
```

或从 [GitHub Releases](https://github.com/martianzhang/apimart-cli/releases) 下载预编译二进制。

### 如何配置 API Key？

支持三种方式（优先级从高到低）：

1. `--api-key` 命令行参数
2. `OPENAI_API_KEY` 或 `APIMART_API_KEY` 环境变量
3. `~/.config/aigc-cli/config.yaml` 配置文件

详见 [installation.md](installation.md)。

### 配置文件放在哪里？

两个位置均可（优先级从高到低）：
- `~/.config/openai/config.yaml`
- `~/.config/aigc-cli/config.yaml`

也可通过 `--config /path/to/config.yaml` 指定。

完整示例见 [config.example.yaml](config.example.yaml)。

## 使用问题

### 支持哪些模型？

任意 OpenAI 兼容格式的 API 均可使用。支持的类型：

| 能力 | 命令 | 说明 |
|---|---|---|
| 图片生成 | `image` | GPT-Image、DALL-E、Grok Imagine 等 |
| 视频生成 | `video` | 豆包 Seedance、VEO3 等 |
| Midjourney | `midjourney` / `mj` | MJ 全系列（imagine、blend、upscale 等） |
| AI 对话 | `chat` | GPT、Claude、Gemini、DeepSeek 等 |

查看完整模型列表：
```bash
aigc-cli models
aigc-cli models --type image
aigc-cli models --type video
```

### 模型名必须写完整吗？有没有默认值？

没有硬编码的默认模型。模型名必须在以下之一设置：
- `--model` 参数
- `defaults.image.model` / `defaults.video.model` / `defaults.chat.model`（config.yaml）
- chat 模式下不传则使用 API 服务端默认值

### 图片/视频生成后保存在哪里？

默认保存到当前目录，可通过 `--output` / `-o` 参数或 `config.yaml` 中的 `output_dir` 指定：

```bash
aigc-cli image --prompt "cat" --output ./downloads
```

### 支持代理吗？

支持。三种方式：

```bash
# 命令行
aigc-cli image --prompt "cat" --http-proxy "http://127.0.0.1:7890"

# 环境变量
export HTTP_PROXY="http://127.0.0.1:7890"

# 配置文件
# http_proxy: "http://127.0.0.1:7890"
```

支持 http、https、socks5 协议。

### 支持从文件读取提示词吗？

支持。`--prompt` 自动识别文件路径和 stdin：

```bash
aigc-cli image --prompt prompt.txt
echo "a cat" | aigc-cli image
aigc-cli image < prompt.txt
```

### 支持 JSON 输入吗？

支持。所有生成命令都支持 `--json`：

```bash
aigc-cli image --json '{"prompt":"a cat","n":2}'
aigc-cli image --json request.json
cat request.json | aigc-cli image --json -
```

### 如何查看即将发送的请求而不实际调用 API？

使用 `--dry-run` 参数：

```bash
aigc-cli image --prompt "test" --dry-run
aigc-cli video --prompt "test" --duration 4 --dry-run
aigc-cli mj imagine --prompt "test" --dry-run
```

### 如何查看更多详细信息？

使用 `--verbose` / `-v` 参数，会打印请求 JSON、响应结果、token 统计等：

```bash
aigc-cli image --prompt "cat" -v
aigc-cli chat --message "hello" -v
```

## Midjourney

### MJ 的工作流是什么？

```
imagine → upscale → zoom / pan / inpaint → modal
  ↓         ↓
reroll    variation / high-variation / low-variation
```

1. **imagine** — 生成 4 张图的网格
2. **upscale** — 选择一张放大
3. 后续可 zoom（扩图）、pan（平移）、inpaint（局部重绘）
4. variation 可从原网格生成变体
5. reroll 重新生成网格

### Inpaint 为什么需要两步？

MJ 的局部重绘分两步：
1. `midjourney inpaint` — 指定源任务，进入 MODAL 状态
2. `midjourney modal` — 提交遮罩和提示词

MODAL 状态最长保持 30 分钟，超时自动取消。

### 如何查看任务支持的后续操作？

使用 `query` 查看任务详情，结果中的 `buttons` 列表显示了所有可用的后续操作：

```bash
aigc-cli mj query task_xxx
```

每个 button 的 `customId` 可直接传给 `--custom-id` 跳过自动匹配。

## MCP

### MCP 是什么？

MCP（Model Context Protocol）是一种让 AI 代理（Claude、Cursor 等）直接调用工具的标准协议。aigc-cli 作为 MCP Server 运行后，AI 代理可以直接生成图片、查询模型等。

### 如何配置 MCP？

```json
{
  "mcpServers": {
    "apimart": {
      "command": "aigc-cli",
      "args": ["mcp"]
    }
  }
}
```

详见 [mcp.md](mcp.md)。

### MCP 工具的 API Key 如何配置？

MCP 模式复用 CLI 的配置体系：
- `~/.config/aigc-cli/config.yaml`
- `OPENAI_API_KEY` 环境变量
- `--api-key` 参数

部分工具（list_models、get_model_pricing）无需 API Key。

## 常见错误

### "API key is required"

未配置 API Key。请设置 `OPENAI_API_KEY` 环境变量或通过 `--api-key` 传入。

### "model is required"

未指定模型名。请使用 `--model` 或在 config.yaml 的 `defaults.*.model` 中设置。

### "API returned status 401"

API Key 无效。检查 Key 是否正确、是否已过期、是否有对应模型的访问权限。

### "polling timed out"

任务处理超时。APIMart 异步任务最长等待 180 秒，某些高分辨率或高画质任务可能需要更长时间。超时不代表任务失败，可以用 `aigc-cli task <task_id>` 手动查询。

## 兼容性

### 支持 OpenAI 吗？

支持。设置 `OPENAI_API_KEY` 环境变量即可（图片生成走同步模式）。

### 支持 OpenRouter 吗？

支持。

```bash
export OPENAI_API_KEY="sk-or-xxx"
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"
```

### 支持自定义中转服务吗？

支持。设置 `OPENAI_BASE_URL` 为任意 OpenAI 兼容 API 地址即可。

## 提示词数据

### 搜索的提示词灵感来自哪里？

`aigc-cli ideas` 搜索的提示词数据来源于三个渠道：

| 来源 | 数量 | 许可 |
|---|---|---|
| [NeXra awesome-ai-image-prompts](https://github.com/NeXra-AI/awesome-ai-image-prompts) | ~12,000 | Apache 2.0 |
| [YouMind](https://youmind.com) 社区提示词 | ~11,000+ | 社区贡献，个人学习参考 |
| [OpenNana](https://opennana.com) | ~11,600 | 公开 API，保留 source_url |

每条数据均保留 `source_url` 字段指向原始出处。**提示词原文版权归原作者所有。**

### 我是原作者，如何移除我的提示词？

请提交 [GitHub Issue](https://github.com/martianzhang/apimart-cli/issues) 注明需要移除的内容，我们会在 7 个工作日内处理。建议附上 `source_url` 或数据中的 `title` 以便定位。

### 这些数据能用于商业用途吗？

**不建议。** 本数据集的初衷是个人灵感搜索和学习参考。大部分数据的版权归属原作者，未取得商业使用授权。如需商用，请直接联系数据源平台或原作者。

## 费用

### 怎么知道一次生成花了多少钱？

图片和视频生成完成后会显示费用信息：
```
Completed in 12s | Cost: $0.00144 (0.0100 credits)
```

chat 使用 `-v` 参数也会显示费用（OpenRouter 支持）。

### 如何查看余额？

```bash
aigc-cli balance           # 当前 API Key 余额
aigc-cli balance user      # 用户账号总余额
```

### 性价比最高的配置？

图片（gpt-image-2-official）最低 $0.00144/张：
```bash
aigc-cli image --prompt "cat" --size "3:1" --resolution "1k" --quality "low"
```

视频（doubao-seedance-2.0）最低 $0.0224/个：
```bash
aigc-cli video --prompt "cat" --duration 4
```

详见各模型的 pricing 页面。
