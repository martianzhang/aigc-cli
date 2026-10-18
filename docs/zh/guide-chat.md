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
aigc-cli chat

# 设置系统提示词后进入交互式
aigc-cli chat --system "你是一位诗人"

# 指定模型进入交互式
aigc-cli chat --model claude-sonnet-4-20250514

# 交互式 + 显示 token 消耗和耗时
aigc-cli chat -v
```

> 交互模式现在使用 **Bubble Tea TUI**（全屏终端界面），替代了旧版的原始终端模式。
> 提供更好的视觉体验、实时状态反馈和工具调用可视化。

### TUI 界面布局

进入交互模式后，终端会切换到全屏 TUI，包含以下区域：

```
┌──────────────────────────────────────┐
│ 💬 Chat with AI — Model: deepseek-v4  │  ← 顶部栏
├──────────────────────────────────────┤
│                                      │
│  You: 1+1等于几？                    │  ← 消息区
│  ─────────────────────────────────   │     （可滚动画板）
│  Assistant: 2                        │
│                                      │
│  🛠️  generate_image:                 │
│  ✓ Successfully generated 1 image(s) │
│                                      │
├──────────────────────────────────────┤
│ 输入消息... (Enter to send)            │  ← 输入框
├──────────────────────────────────────┤
│ ● Ready | Provider: apimart | Model: deepseek-v4 | F1 │  ← 状态栏
└──────────────────────────────────────┘
```

### 命令与快捷键

| 操作 | 说明 |
|---|---|
| `/exit` `/quit` `/q` | 退出 |
| `Ctrl+C` `Ctrl+D` | 退出 |
| `Esc` | 取消正在进行的 API 调用 |
| `F1` `/help` `/`?` | 显示帮助 |
| `/clear` `/reset` `/new` | 清除对话历史 |
| `/tools` | 列出可用工具 |
| `/<tool> <json>` | 直接调用工具 |
| `/preview <file>` | 预览图片/视频 |
| `@path` | 引用本地文件：文本内联、图片作为附件（`Tab` 补全路径；受 `defaults.chat.read_deny` 限制） |
| `PgUp` `PgDn` / 鼠标滚轮 | 上下滚动对话输出（选中文本需按住终端「绕过鼠标报告」的键再拖拽：**iTerm2 是 `⌥ Option`**，多数终端是 `Shift`；`/copy` 可复制最后一条回复） |
| `!<command>` | 执行 Shell 命令（输出区会带 `$ 命令` 前缀，便于看清执行了什么） |

> **多行输入：** 按 `Alt+Enter` 或 `Ctrl+J` 换行，`Enter` 提交消息。
> 输入框最高支持 5 行，超出会自动滚动。

### 交互示例

```bash
# 直接进入交互式 TUI 对话
$ aigc-cli chat
```

在 TUI 中输入消息后，AI 回复将流式逐 token 实时显示。工具调用时，状态栏会显示
spinner 和进度信息。

`-v` 模式下每轮结束后额外显示耗时、token 和费用（输出到 stderr，不影响对话内容）：

```
$ aigc-cli chat -v
>>> 你好
你好！有什么可以帮助你的？
---  Model: deepseek-v4-flash  |  Tokens: 15↑ + 8↓ = 23  |  Cost: $0.000001  |  Time: 1.234s
>>>
```

## 文件引用（@）

在消息里用 `@path` 引用本地文件，不必手动把内容贴进对话。

```bash
# 交互式：输入 @ 后按 Tab 补全路径
aigc-cli chat
>>> 帮我 review @internal/cli/chat/chat.go

# 非交互式同样支持
aigc-cli chat --message "解释 @main.go"
```

| 文件类型 | 行为 |
|---|---|
| 文本文件 | 原样内联进消息：`<file name="路径">内容</file>` |
| 图片（png / jpeg / gif / webp） | 作为图片附件发送（base64），正文保留 `[attached image: 路径]` 标记；需使用支持视觉的模型 |
| 二进制文件 | 不内联，原样保留 `@路径` 并给出警告 |
| 超过 1 MiB 的文本 / 超过 20 MiB 的图片 | 拒绝内联或附加，给出警告 |
| 解析不到的 token | 原样保留（`@用户名`、`@scope/pkg` 不会被误当成路径） |

**Tab 补全：** 输入 `@` 后按 `Tab` 循环匹配项目文件。git 仓库内走 `git ls-files`（遵循 `.gitignore`），否则回退为跳过隐藏条目的文件遍历。

**安全边界：**

- `@` 读取与 `read` 工具共用同一套策略：`defaults.chat.read_deny` 中的正则一样会拦截，被拦截时**不会**读取文件，只给出警告。
- 二进制文件不会作为文本内联，避免把乱码灌进上下文。
- 仅对**用户消息**生效；工具调用参数里的 `@` 由另一套逻辑展开（见下文 `read` 原语）。

## Agentic Chat — 聊天中生成图片和视频

Chat 默认启用 Agent Loop，LLM 可以调用 `generate_image` 和 `generate_video` 工具来生成图片和视频。交互式和非交互式模式均支持。

```bash
# 交互式：直接让 LLM 生成图片
aigc-cli chat
>>> 画一只猫在星空下
# LLM 调用 generate_image → 图片保存到本地 → LLM 告知用户保存位置

# 非交互式单轮：一条消息搞定
aigc-cli chat --message "画一只猫，然后写首诗描述它"
# LLM 调 generate_image → 生成图片 → 继续 → 输出诗 → 结束

# 生成视频
aigc-cli chat --message "生成一段日落海滩的视频"
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

### 可用工具（原语）

Chat 只内置少量**原语**（命名对齐 pi coding agent）；生成 / 检测 / 知识库等**领域能力改由 MCP 服务器提供**（见下文「连接外部 MCP 服务器」）。

| 原语 | 默认 | 说明 |
|---|---|---|
| `read` | ✅ | 读取本地文本文件（支持 `offset`/`limit` 分页） |
| `grep` | ✅ | 按正则搜索文件内容 |
| `find` | ✅ | 按文件名 / glob 递归查找 |
| `ls` | ✅ | 列出目录条目（非递归） |
| `write` | ⛔ | 写入 / 覆盖文件，自动建父目录 |
| `edit` | ⛔ | 精确文本替换（`old_string` 必须唯一匹配） |
| `bash` | ⛔ | 在当前目录执行 shell 命令 |
| `search` | ⛔ | 内置联网搜索（DuckDuckGo，免费免 Key），返回标题 + URL 列表 |
| `fetch` | ⛔ | 内置抓取 URL：HTML 转 Markdown（正文提取，省 token），非 HTML 原样返回；免费、带 SSRF 防护 |
| `web_search` | ⛔ | 经**第三方 API** 搜索（厂商由 `search_provider` 选择，可能计费），返回标题 + URL 列表 |
| `web_fetch` | ⛔ | 经**第三方 API** 抓取（同一厂商，返回已格式化 Markdown，可能计费） |

**只读原语**（`read`/`grep`/`find`/`ls`）默认开启；**特权原语**（`write`/`edit`/`bash`/`search`/`fetch`/`web_search`/`web_fetch`）默认关闭，需在 `tools_enable` 中**显式列出**才启用——这是把写文件 / 执行命令 / 联网交给模型的安全闸门。

领域工具（`generate_image`、`generate_video`、`recognize_text`、`kb_find` 等）不再定义在 Chat 内，而由 MCP 服务器提供：想用 aigc-cli 自己的能力，配置 `mcp.servers` 指向 `aigc-cli mcp` 即可（见下）。

> `read` 默认可以读取**任意文件**。如需禁止读取敏感文件，用 `defaults.chat.read_deny` 配置**文件名/路径正则黑名单**（Go regexp，匹配路径或文件名即拒绝），例如 `read_deny: ["\\.pem$", "^id_rsa$", "\\.env$"]`；留空则全部放行。
>
> **二进制防护**：`read` 会嗅探内容，含 NUL 字节或大量非法 UTF-8 的文件（图片、可执行文件等）不会作为文本返回，而是提示 `[binary file — not displayed as text]`，避免把乱码灌进上下文。
>
> **`fetch` 的正文提取**：内置 `fetch` 抓到的 HTML 会先用 readability 提取正文、再转成 Markdown（丢弃导航/页脚等样板，显著省 token）；提取不到正文时回退为整页转 Markdown。非 HTML（JSON、纯文本等）原样返回。结果按**字符**（非字节）分页：`max_length` 默认 5000、最大 50000，超大结果用返回里给出的 `offset` 继续读。
>
> 内置 `fetch` 免费，并带 **SSRF 防护**：请求前拒绝回环 / 私网 / 链路本地 / 元数据地址（`169.254.169.254` 等），重定向逐跳校验。第三方 `web_fetch` 的请求发往厂商服务器，安全性由厂商策略负责。

### 联网：内置 vs 第三方

Chat 有两套联网原语，命名规则是——**不带 `web_` = 内置免费实现；带 `web_` = 走第三方 API（可能计费）**：

| 能力 | 内置（免费） | 第三方 API（按厂商计费） |
|---|---|---|
| 搜索 | `search`（DuckDuckGo 抓取） | `web_search`（厂商由 `search_provider` 选择） |
| 抓取 | `fetch`（本地 HTTP + readability） | `web_fetch`（同一厂商的抓取接口） |

四个都是**特权原语**，需在 `tools_enable` 中显式开启：

```yaml
tools_enable:
  - "search"
  - "fetch"
  - "web_search"
  - "web_fetch"
```

内置的 `search`/`fetch` **无需任何配置、不产生费用**。第三方的 `web_search`/`web_fetch` 由顶层 `web_search.*` 中声明的厂商提供，用 `defaults.chat.search_provider` 选择**厂商**。厂商一般同时提供 Search 与 Fetch，所以两者**共用同一个厂商选择**，没有单独的 fetch 配置：

```yaml
web_search:
  deepseek:
    type: deepseek             # 自研搜索引擎，同时提供 Search 与 Fetch
    api_key: "sk-xxx"          # 0.5 分/次成功请求（Search、Fetch 同价）
  doubao:
    type: doubao
    api_key: "your-doubao-api-key"
  firecrawl:
    type: firecrawl
    api_key: "fc-xxx"
    tags: [free, quality]

defaults:
  chat:
    search_provider: deepseek   # duckduckgo/firecrawl/brave/doubao/deepseek，或 auto/free/quality（默认 auto）
```

- `search_provider` 取值：厂商名指定该厂商；`auto`（默认）按 `free`/`quality` 标签选择；`free`/`cheap` 只用免费引擎；`quality` 只用高质量引擎。
- 也可用 `--search-provider` 命令行参数临时覆盖配置（优先级：`--search-provider` > `defaults.chat.search_provider` > `auto`）。
- `web_search` **只使用第三方厂商**：内置 DuckDuckGo 不参与，避免 `auto` 时静默回落成"非第三方"。未声明任何第三方厂商时，`web_search`/`web_fetch` 会**报错并提示改用内置 `search`/`fetch`**（不会静默花钱，也不会静默降级）。
- `web_fetch` 需要**具备抓取能力的厂商**（如 `deepseek`）；只有搜索能力的厂商（`duckduckgo`/`brave`）不提供 `web_fetch`。
- 单次调用默认返回 5 条、最多 10 条。
- 与 `kb search` 不同，chat 的联网原语**只搜索/抓取、不入库**；`kb search` 仍使用同一套 `web_search.*` 厂商配置（含免费 DuckDuckGo 兜底）。

### 项目上下文（AGENTS.md）

启动时 Chat 会自动加载 `AGENTS.md` 作为**项目上下文**，追加到 system prompt：

- 全局：`~/.config/aigc-cli/AGENTS.md`（对所有会话生效）；
- 项目：从当前目录**逐级向上**查找 `AGENTS.md`，直到仓库根（首个含 `.git` 的目录）；多个文件按「最外层 → 最内层」拼接，越靠内越具体。

没有则跳过。适合放项目约定、构建/测试命令、代码风格等。

### 交互式快捷键

在交互式 REPL 中支持以下快捷操作：

| 操作 | 说明 |
|---|---|---|
| `/<tool_name> <json>` | 直接调用工具，如 `/generate_image {"prompt":"a cat"}` |
| `!<shell_cmd>` | 执行系统命令，如 `!ls -la`（30s 超时）；结果区会回显 `$ 命令` 再附输出 |
| `/preview <file>` | 用系统默认程序打开图片/视频 |
| `/tools` | 列出所有可用工具 |
| `/help` | 显示帮助 |

### 配置工具白名单/黑名单

顶层 `tools_enable` / `tools_disable` 控制**特权原语**的开启，以及 MCP 服务器对外暴露的工具面；`defaults.chat.tools` / `disable_tools` 在「原语 ∪ MCP」并集上再收窄：

```yaml
# 顶层：显式启用特权原语（只读原语 read/grep/find/ls 默认即可用）
tools_enable:
  - "read"
  - "grep"
  - "find"
  - "bash"        # 显式开启 shell
  - "write"       # 显式开启写文件
  - "web_search"  # 第三方搜索 API（按厂商计费）
  - "web_fetch"   # 第三方抓取 API（按厂商计费）
  # - "search"    # 内置联网搜索（DuckDuckGo，免费免 Key）
  # - "fetch"     # 内置抓取（本地 readability，免费）

# 顶层黑名单，永远优先
tools_disable:
  - "remove_watermark"

defaults:
  chat:
    tools: ["*"]                 # 并集上再收窄（glob；空或 ["*"] = 全部）
    # disable_tools: ["grep"]    # 禁用某个原语
```

规则要点：

- **只读原语**（`read`/`grep`/`find`/`ls`）默认可用，`tools_disable` 可关掉。
- **特权原语**（`write`/`edit`/`bash`/`search`/`fetch`/`web_search`/`web_fetch`）只有在 `tools_enable` 里**显式列出**才启用。
- 领域工具（MCP 来源）在有 `tools_enable` 时按白名单过滤，为空时全部允许。
- `tools_disable` 永远优先于 `tools_enable`。

### 连接外部 MCP 服务器（Chat 作为 MCP 客户端）

Chat 可以作为 **MCP 客户端**，连接任意 stdio MCP 服务器，把它们提供的工具并入 Agent 的工具集。**默认没有任何内置服务器**——想用 `aigc-cli` 自己的 AIGC 工具，就把 `aigc-cli mcp` 配成一个普通 server：

```yaml
mcp:
  servers:
    # 用 aigc-cli 自己的 AIGC 工具
    - name: aigc
      command: aigc-cli                    # 不在 $PATH 时用绝对路径
      args: ["mcp"]
      # tools_enable: ["generate_*"]        # 仅暴露匹配工具（glob，空=全部）
      # tools_disable: ["remove_watermark"] # 排除工具（优先于 enable）
      # enabled: true                       # 默认 true
      # env: ["KEY=VALUE"]                  # 追加/覆盖子进程环境变量
    # 第三方示例
    - name: playwright
      command: npx
      args: ["-y", "@playwright/mcp"]
      tools_enable: ["browser_*"]
```

| 字段 | 说明 |
|---|---|
| `name` | server 标识（日志/报错用）；留空回退到 `command` |
| `command` / `args` | 启动命令（目前仅支持 **stdio**） |
| `env` | 追加/覆盖子进程环境变量（`KEY=VALUE`），父进程环境自动继承 |
| `tools_enable` / `tools_disable` | **仅 Chat 端**过滤该 server 暴露给模型的工具（glob；disable 优先） |
| `enabled` | 是否连接（默认 `true`） |

- server 级的 `tools_enable`/`tools_disable` **只影响 Chat 看到哪些工具**，不影响该 MCP 服务器自身对外暴露的工具面（那由顶层 `tools_enable`/`tools_disable` 控制）。
- 合并后的工具集仍受顶层 `tools_enable`/`tools_disable` 和 `defaults.chat.tools`/`disable_tools` 约束。
- 某个 server 连接失败（命令不存在、初始化超时等）会被**跳过**并在 stderr 提示，不影响其它 server 与正常对话。
- 连接在启动时建立、退出时关闭；`/tools` 会列出所有可用工具（含 MCP 来源的）。
- 工具返回的图片/音频以**本地文件路径**呈现给模型（不内联图像）；生成的媒体已落盘，模型据此告知用户文件位置。

### 工具按需加载（可选）

默认情况下，所有已连接 MCP 服务器的工具都会随每次请求一起发送（schema 常驻上下文）。工具很多、上下文吃紧时，可开启按需加载：

```yaml
defaults:
  chat:
    tools_on_demand: true
```

开启后，请求里只发送**原语**加两个元工具；模型先用 `search_tools`（按关键词返回工具名 + 一行简介）找到需要的工具，再调用 `load_tools` 加载——被加载工具的完整 schema 才会在**后续轮次**进入上下文。原语始终可用，不受影响。

### 参数

| 参数 | 说明 |
|---|---|
| `defaults.chat.max_iterations` | 每次用户消息，LLM 最多连续调工具次数（默认 10） |
| `defaults.chat.tools` | 允许的工具白名单（glob 模式），空或 `["*"]` = 全部允许 |
| `defaults.chat.disable_tools` | 禁用的工具黑名单（glob 模式），覆盖 `tools` |
| `defaults.chat.allow_tool_override` | `true`=LLM参数优先，`false`=配置强制覆盖LLM（默认`false`，省钱） |
| `defaults.chat.tools_on_demand` | `true`=MCP 工具按需加载（`search_tools`/`load_tools`），降低上下文占用（默认 `false`） |
| `defaults.chat.read_deny` | `read` 原语的拒绝正则列表（匹配文件路径或文件名）；空 = 全部放行 |
| `defaults.chat.search_provider` | `web_search`/`web_fetch` 原语使用的第三方厂商（`duckduckgo`/`firecrawl`/`brave`/`doubao`/`deepseek`，或 `auto`/`free`/`quality`）；空 = `auto` |

---

## 非交互式单轮

传 `--message` 或通过管道传入内容时为单次请求模式：

```bash
# 基本对话（流式输出）
aigc-cli chat --message "你好，请介绍一下自己"

# 系统提示词
aigc-cli chat --system "你是一位诗人" --message "写一首关于AI的诗"

# 多轮对话（多个 --message）
aigc-cli chat \
  --message "什么是机器学习？" \
  --message "能举个例子吗？"

# 非流式输出
echo "Explain Go in 3 words" | aigc-cli chat --no-stream

# 指定模型
aigc-cli chat --model gpt-4o --message "Hello"

# 单轮模式查看 token 消耗和耗时
aigc-cli chat --message "hi" -v
```

单轮模式下 `-v` 输出示例：

```
$ aigc-cli chat --message "hi" -v
Hello! How can I help you today?
---  Model: deepseek-v4-flash  |  Tokens: 15↑ + 8↓ = 23  |  Cost: $0.000001  |  Time: 1.234s
```

### 使用 OpenRouter

```bash
export OPENAI_API_KEY="sk-or-xxx"
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"

aigc-cli chat --model "openai/gpt-4o" --message "Hello"
```

### 使用任意 OpenAI 兼容中转

```bash
export OPENAI_API_KEY="sk-xxx"
export OPENAI_BASE_URL="https://your-relay.com/v1"

aigc-cli chat --message "Hello"
```

## 参数

| 参数 | 说明 |
|---|---|
| `--message` | 用户消息（可重复，实现多轮对话）。**不传此参数时自动进入交互式多轮模式** |
| `--system`, `-s` | 系统提示词，设定 AI 角色 |
| `--model`, `-m` | 模型名（全局 flag，不传则由各命令使用自己的默认） |
| `--temperature`, `-t` | 采样温度 0-2，默认 1.0 |
| `--max-output` | 最大生成 token 数 |
| `--context-size` | 输入上下文窗口大小（token），0=使用模型默认值。对话超过其 80% 时自动压缩（总结早期消息） |
| `--no-stream` | 关闭流式输出，等待完整响应 |
| `--interactive`, `-i` | 强制进入交互式多轮模式 |
| `--json` | JSON 输入（文件、字符串或 `-` 表示 stdin）；原样转发，显式 CLI 参数覆盖对应键，未建模字段（如 `reasoning_effort`、厂商私有键）原样保留 |
| `--inject-json` | 在 CLI 生成的最终 body 上**追加合并**一段 JSON（对象递归合并、数组拼接、标量覆盖）；用于联网搜索等厂商私有参数，优先级高于 `--json` |
| `--dry-run` | 打印真实端点与请求体的等价 curl，不调用 API |
| `--search-provider` | `web_search`/`web_fetch` 原语使用的第三方厂商（`duckduckgo`/`firecrawl`/`brave`/`doubao`/`deepseek`，或 `auto`/`free`/`quality`），覆盖 `defaults.chat.search_provider` |
| `--verbose`, `-v` | 显示 token 消耗、费用和耗时统计（全局 flag） |

### `--json` 与参数合并

只传 `--json`（不带任何其他参数）时，原文逐字节原样发送。同时给出 CLI 参数时，只有**显式指定**的参数覆盖 body 中对应的键，其余键原样保留：

```bash
# messages 结构照常来自 JSON，只把 max_tokens / stream 换成参数值
aigc-cli chat --json request.json --max-output 1024 --no-stream
```

> 💡 参数到 JSON 键的映射沿用常规的 flag→字段映射（如 `--max-output` → `max_tokens`、`--no-stream` → `stream`）。body 必须是 JSON **对象**；`--json` 支持内联字符串、文件路径或 `-`（stdin），路径文件不存在时明确报 `file not found: <路径>`。

`--json` 也接受 **JSONC**：解析前会剥离 `//` 行注释、`/* */` 块注释和 `}` / `]` 前的尾随逗号，并忽略开头的 UTF-8 BOM；字符串里的 `//` 或 `/*` 原样保留，不含注释的严格 JSON 逐字节不变。真正发往 API 的 body 是去掉注释后的 JSON。

```jsonc
{
  // 结构化参数可以写注释
  "model": "gpt-4o",
  "messages": [{ "role": "user", "content": "你好" }],
  "stream": false, // 尾随逗号也可以
}
```

> 💡 行为类参数永远不写进 body：`--dry-run`、`--provider`、`--api-key`、`--api-base`、`--http-proxy`、`--output`、`--verbose`、`--timeout`、`--config`、`--print-config`，以及 chat 专有的 `--context-size`、`--interactive`、`--search-provider`。

### `--inject-json`：在最终 body 上叠加一段 JSON

`--inject-json` 与 `--json` 平级，是第二套 body 输入通道：把你给的一小段 JSON **合并进** CLI 生成的最终 body，用来传入 CLI 未建模的厂商参数（联网搜索、推理强度、路由偏好等）。

- **合并语义**：对象递归合并；**数组拼接**（CLI 的在前，你的在后）；标量/其它值覆盖。
- **优先级**：typed 字段/CLI 参数 < `--json` < `--inject-json`——`--inject-json` 永远最后生效。
- 输入：文件路径或内联 JSON（**不支持 stdin**——避免和 `--json` / `--prompt` 抢 stdin）；值必须是 JSON **对象**，否则直接报错。
- **已知限制**：数组只能追加、无法清空或替换；值必须是对象，不接受数组或标量。

> 💡 `--inject-json` 的内容是 **provider 相关**的，CLI 不做转换：`{"enable_search":true}` 只对百炼 OpenAI 兼容协议有效；Anthropic 原生用 `{"tools":[{"type":"web_search_20250305","name":"web_search"}]}`；OpenAI Responses 用 `{"tools":[{"type":"web_search_preview"}]}`；OpenRouter 用 `{"plugins":[{"id":"web"}]}`（或模型名后缀 `:online`）。担心写错时先用 `--dry-run` 看待发 curl。

**例：OpenRouter 联网搜索。** OpenRouter 的 `web` 插件对**任意模型**生效（非原生搜索模型走 Exa），它是 chat-completions 的 body 字段，所以 provider 要用 `openai` 类型（走 `/chat/completions`，**不能**用 `type: anthropic`）：

```yaml
providers:
  openrouter:
    type: openai
    api_key: sk-or-xxx
    base_url: https://openrouter.ai/api/v1
```

```bash
aigc-cli chat -P openrouter --model anthropic/claude-sonnet-5.5 \
  --message "明天纽约天气" --no-stream \
  --inject-json '{"plugins":[{"id":"web","max_results":5}]}'
```

`plugins` 还能写 `engine`（`native`/`exa`/`firecrawl`/…）、`include_domains` / `exclude_domains`、`search_prompt`。不想调参时也可用模型名后缀 `:online`（等价于 `plugins:[{"id":"web"}]`）：`--model anthropic/claude-sonnet-5.5:online`。联网按次计费。

### Provider 协议类型

`config.providers.<name>.type` 决定该 Provider 的 `chat` 命令走哪种协议：

| 类型值 | 说明 |
|---|---|
| `openai` | 默认。OpenAI 兼容协议，`POST /v1/chat/completions` |
| `openai_chat_completions` | `openai` 的显式别名，同样是经典 chat completions |
| `openai_responses` | OpenAI Responses API，`POST {base_url}/responses` |
| `anthropic` | Anthropic Messages API |
| `ollama` | Ollama 本地模型 |

`type: openai_responses` 用于部分推理模型（如通过 OpenLux 等中转的 GPT-6 系列）在 `/v1/chat/completions` 上拒绝函数工具的情况。这些模型返回 HTTP 400：“Function tools with reasoning_effort are not supported ... use /v1/responses or set reasoning_effort to 'none'”。将 Provider 类型设为 `openai_responses` 后，`chat` 会改用 Responses API 发送请求，从而支持工具调用与推理并存。

配置示例：

```yaml
providers:
  openlux:
    type: openai_responses
    api_key: sk-xxx
    base_url: https://api.openlux.ai
defaults:
  chat:
    provider: openlux
    model: gpt-6-luna
```

其他命令（image / video / audio / music）不受 `chat` 协议类型影响。

使用 `--json` 时，请求体原样转发。若 Provider 类型为 `openai_responses`，JSON 内容需符合 Responses API 格式（使用 `input` / `instructions` 等字段），而非 chat completions 的 `messages` 格式。

### 零数据保留（ZDR）

`--zdr` 请求服务商不保留任何数据。当前只有 OpenRouter 提供请求级开关，且**仅作用于其 chat-completions 端点**（`chat`，以及走 chat/completions 的 OpenRouter `music`）：

```bash
export OPENAI_API_KEY="sk-or-xxx"
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"

aigc-cli chat --zdr --message "你好"
# 请求体包含："provider": {"zdr": true, "data_collection": "deny"}
```

- 优先级：`--zdr` 参数 > `providers.{name}.zdr` > 全局 `zdr` > 关闭。只有部分账号支持 ZDR 时用 Provider 级别配置：
  ```yaml
  providers:
    openrouter:
      base_url: "https://openrouter.ai/api/v1"
      zdr: true
  ```
- 开启后 OpenRouter 只会路由到符合 ZDR 的上游；若无可用上游，请求会**直接失败**（HTTP 404 “No endpoints found matching your data policy”），而不会静默回退到非 ZDR 服务商。
- `--json` body 中已有的其他 `provider` 字段会保留，仅强制写入 `zdr` 与 `data_collection`。
- OpenRouter 的**图片 / 视频** API 不接受 `zdr`（视频按设计不支持 ZDR），因此 `--zdr` 在 `image` / `video` 上是静默空操作，绝不会注入不受支持的字段。
- 其他 Provider 没有请求级 ZDR 开关（留存策略需在服务商控制台/账户侧配置），`--zdr` 同样为空操作。

### 上下文管理

交互模式中，当历史对话接近 `--context-size` 限制时（超过 80%），会自动将早期消息总结为一条摘要以释放空间，无需手动干预。你也可以随时输入 `/compact` 手动触发压缩。

> **关于模型**：代码不硬编码默认模型名，仅当用户通过 `--model` 或配置文件指定时才传入模型参数；否则由 API 服务端自行决定使用哪个模型。这样即使模型迭代频繁，也无需更新 CLI。 |
