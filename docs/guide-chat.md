# AI 对话

支持流式输出（默认），完全兼容 OpenAI 格式，支持 OpenRouter、OpenAI、以及任意第三方中转。
可使用 GPT、Claude、Gemini、DeepSeek 等模型。**不硬编码默认模型**，由 API 服务端决定。

支持两种模式：
- **交互式多轮**（默认）：不带 `--message` 时自动进入交互式 REPL，多轮连续对话
- **非交互式单轮**：带 `--message` 或通过管道传入内容，一次请求即结束

## 交互式多轮（默认）

不传 `--message` 时自动进入交互模式：

```bash
# 直接进入交互式对话
apimart-cli chat

# 设置系统提示词后进入交互式
apimart-cli chat --system "你是一位诗人"

# 指定模型进入交互式
apimart-cli chat --model claude-sonnet-4-20250514

# 交互式 + 显示 token 消耗和耗时
apimart-cli chat -v
```

支持以下命令和快捷操作：

| 操作 | 说明 |
|---|---|
| `/exit` `/quit` `/q` | 退出 |
| `exit` `quit` `bye` | 同上（无需 `/`） |
| `退出` `再见` | 同上（中文） |
| `Ctrl+C` | 退出 |
| `Ctrl+D` | 退出（所有平台，原始终端模式） |
| `/clear` `/reset` | 清除对话历史 |
| `/help` | 显示帮助 |

交互式对话示例：

```
$ apimart-cli chat
Interactive chat mode. Type /exit or Ctrl+C to quit.

>>> 1+1等于几？
2

>>> 再乘以3呢？
2 × 3 = 6

>>> /exit
Bye!
```

`-v` 模式下每轮结束后额外显示耗时、token 和费用（输出到 stderr，不影响对话内容）：

```
$ apimart-cli chat -v
>>> 你好
你好！有什么可以帮助你的？
---  Model: deepseek-v4-flash  |  Tokens: 15↑ + 8↓ = 23  |  Cost: $0.000001  |  Time: 1.234s
>>>
```

## 非交互式单轮

传 `--message` 或通过管道传入内容时为单次请求模式：

```bash
# 基本对话（流式输出）
apimart-cli chat --message "你好，请介绍一下自己"

# 系统提示词
apimart-cli chat --system "你是一位诗人" --message "写一首关于AI的诗"

# 多轮对话（多个 --message）
apimart-cli chat \
  --message "什么是机器学习？" \
  --message "能举个例子吗？"

# 非流式输出
echo "Explain Go in 3 words" | apimart-cli chat --no-stream

# 指定模型
apimart-cli chat --model gpt-4o --message "Hello"

# 单轮模式查看 token 消耗和耗时
apimart-cli chat --message "hi" -v
```

单轮模式下 `-v` 输出示例：

```
$ apimart-cli chat --message "hi" -v
Hello! How can I help you today?
---  Model: deepseek-v4-flash  |  Tokens: 15↑ + 8↓ = 23  |  Cost: $0.000001  |  Time: 1.234s
```

### 使用 OpenRouter

```bash
export OPENAI_API_KEY="sk-or-xxx"
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"

apimart-cli chat --model "openai/gpt-4o" --message "Hello"
```

### 使用任意 OpenAI 兼容中转

```bash
export OPENAI_API_KEY="sk-xxx"
export OPENAI_BASE_URL="https://your-relay.com/v1"

apimart-cli chat --message "Hello"
```

## 参数

| 参数 | 说明 |
|---|---|
| `--message` | 用户消息（可重复，实现多轮对话）。**不传此参数时自动进入交互式多轮模式** |
| `--system`, `-s` | 系统提示词，设定 AI 角色 |
| `--model`, `-m` | 模型名（全局 flag，不传则由各命令使用自己的默认） |
| `--temperature`, `-t` | 采样温度 0-2，默认 1.0 |
| `--max-tokens` | 最大生成 token 数 |
| `--no-stream` | 关闭流式输出，等待完整响应 |
| `--interactive`, `-i` | 强制进入交互式多轮模式 |
| `--json` | JSON 输入（文件、字符串或 `-` 表示 stdin） |
| `--verbose`, `-v` | 显示 token 消耗、费用和耗时统计（全局 flag） |

> **关于模型**：代码不硬编码默认模型名，仅当用户通过 `--model` 或配置文件指定时才传入模型参数；否则由 API 服务端自行决定使用哪个模型。这样即使模型迭代频繁，也无需更新 CLI。 |
