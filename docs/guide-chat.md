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
|---|---|---|
| `/exit` `/quit` `/q` | 退出 |
| `exit` `quit` `bye` | 同上（无需 `/`） |
| `退出` `再见` | 同上（中文） |
| `Ctrl+C` | 中断当前操作并退出 |
| `Ctrl+D` | 退出 |
| `/clear` `/reset` | 清除对话历史 |
| `/help` | 显示帮助 |

### 多行输入

输入较长的 prompt 时支持两种多行方式：

- **行尾 `\` + Enter**：类 Shell 续行，适合边想边写。`\` 会被替换为换行符，最终作为完整消息提交：
  ```
  >>> 请帮我写一段Python代码，\
  ... 实现一个简单的Web服务器，\
  ... 使用Flask框架
  ```
  输出将为 `请帮我写一段Python代码，\n实现一个简单的Web服务器，\n使用Flask框架`

- **`` ``` `` + 空行提交**：适合粘贴大段文本。输入 `` ``` `` 开头后，每行逐条读取，空行结束并提交：
  ```
  >>> ```
  ... 第一段内容
  ... 第二段内容
  ... （空行）
  ```

交互式对话示例（回复流式输出，逐 token 实时显示）：

```
$ apimart-cli chat
Interactive chat mode. Type /help for commands, /exit or Ctrl+C to quit.
Model: deepseek-v4-flash | Mode: stream

>>> 1+1等于几？
2

>>> 画一只猫在星空下
[tool] generate_image:
  prompt=画一只猫在星空下
  size=16:9
  quality=high
⠋ ░░░░░░░░░░░░░░░░░░░░ 0%
...
[tool] done in 8.5s: Successfully generated 1 image(s)
图片已保存到 output/image_task_xxx.png

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

## Agentic Chat — 聊天中生成图片和视频

Chat 默认启用 Agent Loop，LLM 可以调用 `generate_image` 和 `generate_video` 工具来生成图片和视频。交互式和非交互式模式均支持。

```bash
# 交互式：直接让 LLM 生成图片
apimart-cli chat
>>> 画一只猫在星空下
# LLM 调用 generate_image → 图片保存到本地 → LLM 告知用户保存位置

# 非交互式单轮：一条消息搞定
apimart-cli chat --message "画一只猫，然后写首诗描述它"
# LLM 调 generate_image → 生成图片 → 继续 → 输出诗 → 结束

# 生成视频
apimart-cli chat --message "生成一段日落海滩的视频"
```

### 工作原理

```
用户输入 → LLM 回复（文本或 tool_call）
  ├─ tool_call(generate_image, {prompt:"..."})
  │   → 调厂商 API 生成图片
  │   → 结果回填给 LLM
  │   → LLM 继续对话
  └─ 文本回复 → 输出给用户
```

- 回复采用**流式输出**，文字逐 token 实时显示
- 每次用户消息，LLM 最多连续调 10 次工具，可通过 `defaults.chat.max_iterations` 配置
- 工具执行时读取 `defaults.image` / `defaults.video` 作为 LLM 没指定参数时的兜底
- 每个工具执行后显示耗时和结果摘要（如 `[tool] done in 3.2s: Successfully generated 1 image(s)`）
- 长耗时操作（图片/视频/MJ 生成）支持 `Ctrl+C` 中断，立即返回 prompt

### 可用工具

默认注册以下工具，LLM 会根据对话内容自动调用：

| 工具 | 说明 | 共享函数 |
|---|---|---|
| `generate_image` | AI 图片生成 | `cmd/image.go: generateImageAndSave` |
| `generate_video` | AI 视频生成 | `cmd/video.go: generateVideoAndSave` |
| `midjourney_imagine` | Midjourney 艺术生图 | `cmd/midjourney.go: midjourneySubmitAndGetText` |
| `midjourney_describe` | 图片反推提示词 | 同上 |
| `midjourney_reroll` | 重新生成 MJ 结果 | 同上 |
| `midjourney_video` | MJ 图片转视频 | 同上 |
| `web_fetch` | 抓取网页内容（走代理，15s 超时） | `cmd/chat.go: executeWebFetch` |
| `ideas` | 搜索本地提示词灵感 | `cmd/ideas.go: searchIdeasText` |
| `balance` | 查询 API 余额 | `cmd/balance.go: getBalanceText` |
| `task` | 查询异步任务状态 | `cmd/task.go: queryTaskText` |
| `read_file` | 读取本地文本文件 | `cmd/chat.go: executeReadFile` |

所有工具通过共享函数实现，`chat` 和对应的 CLI 命令走同一份代码，无重复逻辑。

### 交互式快捷键

在交互式 REPL 中支持以下快捷操作：

| 操作 | 说明 |
|---|---|---|
| `/<tool_name> <json>` | 直接调用工具，如 `/generate_image {"prompt":"a cat"}` |
| `!<shell_cmd>` | 执行系统命令，如 `!ls -la`（30s 超时） |
| `/preview <file>` | 用系统默认程序打开图片/视频 |
| `/tools` | 列出所有可用工具 |
| `/help` | 显示帮助 |

### 配置工具白名单/黑名单

```yaml
defaults:
  chat:
    tools: ["*"]                    # 全部允许（默认）
    # tools: ["generate_image"]     # 只允许生成图片
    # disable_tools: ["generate_video"]  # 禁用视频生成
```

`tools` 和 `disable_tools` 支持 glob 模式匹配。

### 参数

| 参数 | 说明 |
|---|---|
| `defaults.chat.max_iterations` | 每次用户消息，LLM 最多连续调工具次数（默认 10） |
| `defaults.chat.tools` | 允许的工具白名单（glob 模式），空或 `["*"]` = 全部允许 |
| `defaults.chat.disable_tools` | 禁用的工具黑名单（glob 模式），覆盖 `tools` |
| `defaults.chat.allow_tool_override` | `true`=LLM参数优先，`false`=配置强制覆盖LLM（默认`false`，省钱） |

---

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
