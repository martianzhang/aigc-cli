# 其他命令

## 检测 AIGC

详见 [docs/guide-detect.md](guide-detect.md)。

## 音乐生成

自然语言生成音乐（APIMart suno/flowmusic 异步、OpenRouter Lyria 同步流式），详见 [guide-music.md](guide-music.md)。

## Shell 补全

生成并启用 shell 命令补全脚本（bash/zsh/fish/powershell）：

```bash
# Bash
source <(aigc-cli completion bash)

# Zsh
source <(aigc-cli completion zsh)

# Fish
aigc-cli completion fish | source

# 持久化安装（写入 shell rc，每次登录自动生效）
echo 'source <(aigc-cli completion bash)' >> ~/.bashrc
```

启用后，输入 `aigc-cli im+Tab` 自动补全为 `aigc-cli image`，输入 `aigc-cli --mod+Tab` 自动补全为 `--model`，无需记忆全部命令和参数名。

## 查询模型列表

支持三个数据源，自动根据 API 地址选择：

| base_url | `--type` 行为 | `--price` 行为 | 无参数行为 |
|---|---|---|---|
| APIMart 域名 | `GET /api/marketplace/models?type=...` | APIMart 定价 API | `GET /v1/models` |
| OpenRouter 域名 | `GET /v1/images\|videos/models`（能力发现） | — | `GET /v1/models` |
| 其他（OpenAI 等） | `GET /v1/models` | — | `GET /v1/models` |

```bash
# 自动选择数据源
aigc-cli models

# APIMart 市场（按类型筛选）
aigc-cli models --type image
aigc-cli models --type video
aigc-cli models --type chat

# APIMart 特定模型定价
aigc-cli models --price gpt-image-2-official

# OpenRouter 模型发现（免认证，无需 API Key）
# 自动调用 /v1/images/models 或 /v1/videos/models
aigc-cli models --type image   # 展示架构、参数、能力
aigc-cli models --type video

# 查询单个模型详情（展示上下文窗口、定价、模态、支持参数）
aigc-cli models --provider openrouter openai/gpt-4o

# OpenAI 标准模型列表
aigc-cli models --api-base "https://api.openai.com/v1"
```

> **单模型查询**：`aigc-cli models <模型名>` 展示单个模型的详情。OpenRouter 走其**单数端点** `GET /v1/model/{author}/{slug}`（与列表的复数 `/models` 不同），并**原样输出接口返回的 JSON（仅格式化缩进，不做字段解析）**；其他 Provider 直接拉取 `GET /v1/models` 列表并按模型名过滤（不再探测 `GET /v1/models/{id}`——多数 Provider 并不支持该端点），无需额外配置。

> **API Key**：无参数调用（或查询单个模型，如 `aigc-cli models gpt-4o`）会请求 `/v1/models`，需要 API Key。未指定 `--provider` 且全局未配置 `api_key` / `OPENAI_API_KEY` 时，命令会直接报错并提示已配置密钥的 Provider；用 `--provider <name>` 指定 Provider，或通过 `--api-key` / 环境变量 `OPENAI_API_KEY` 提供密钥。`--type` / `--price`（市场与定价）免认证，无需 API Key。

## 查询任务状态

仅 APIMart 异步模式可用：

```bash
# 使用全局配置的 Provider
aigc-cli task task_01KV4KD9FBH3AZ4DE18A7Y17S3

# 使用指定 Provider 的账号与接口地址查询
aigc-cli task --provider apimart task_01KV4KD9FBH3AZ4DE18A7Y17S3
```

`--provider <name>` 决定用哪个 Provider 的账号与接口地址查询，被查询的任务必须属于该 Provider。未指定且全局未配置密钥时，命令会直接报错并提示已配置密钥的 Provider。

返回完整的任务信息（状态、进度、耗时、费用、结果 URL 等）。图片任务完成后自动下载图片到 `--output` 目录。

## 查询余额

查询各 Provider 的账户余额：

```bash
# 查询所有已配置且带 API Key 的 Provider
aigc-cli balance

# 查询指定 Provider
aigc-cli balance --provider siliconflow

# 查询用户账号的总余额
aigc-cli balance user
```

> **API Key**：未指定 `--provider` 时，会查询所有已配置且带 API Key 的 Provider（Ollama 等本地 Provider 免密钥）。显式指定但未配置密钥的非本地 Provider 会直接报错并给出提示；用 `--provider <name>` 指定已配置密钥的 Provider。

## 配置读写（config）

无需手动编辑 YAML，即可读取和修改 `config.yaml`。键使用与 YAML 结构一致的点号路径：

```bash
# 读取单个值（密钥自动脱敏）
aigc-cli config get defaults.image.model

# 读取整个段落（嵌套的密钥同样脱敏）
aigc-cli config get providers

# 写入单个值：原子替换，原文件备份为 config.yaml.bak
aigc-cli config set defaults.image.model gpt-image-2

# 查看当前生效配置（密钥脱敏）
aigc-cli config list
```

说明：

- 配置文件解析规则与其他命令一致：优先 `--config <path>`，否则 `~/.config/aigc-cli/config.yaml`。文件不存在时 `get`/`set` 直接报错，**绝不自动创建文件**。
- `set` 只替换叶子值。点号路径中缺少父级段落时会报错（`set defaults.chat.allow_tool_override: section not found`），**绝不自动创建段落**。
- 保留已有键的 YAML 类型：原本是 int/bool/float 就继续保持该类型，无法解析的值会被拒绝；原本是字符串的仍是字符串。在已有段落中新建的键，除纯整数或 `true`/`false` 外都按字符串写入。
- 写入是原子的：先把原文件复制为 `<path>.bak`，新内容写入 `<path>.tmp.<pid>`，再用 rename 覆盖目标文件。注释、键顺序、标量引号风格都会保留；空行与个别空格的排版可能被规范化。
- 任何情况下都不会完整打印密钥：`api_key` 只显示后 4 位（`...abcd`），`base_url` / `http_proxy` 中的凭据显示为 `REDACTED`，与 `--print-config` 的脱敏规则一致。
- 写入 `api_key` / `base_url`（全局或 `providers.*` 下）必须加 `--force`，因为它们决定密钥被发送到何处：

```bash
aigc-cli config set api_key sk-xxx
# Error: refusing to set api_key without --force: api_key/base_url hold credentials or endpoint overrides

aigc-cli config set api_key sk-xxx --force
```

- 配置文件不存在时 `config list` 依然可用：会打印一行 `# config file not found` 注释，再输出代码默认值。

## Dry-run 调试

打印即将提交的 curl 命令，不实际调用 API：

```bash
# 图片 dry-run
aigc-cli image --prompt "test" --size "16:9" --dry-run

# 视频 dry-run
aigc-cli video --prompt "test" --duration 4 --dry-run

# Midjourney dry-run
aigc-cli mj imagine --prompt "test" --dry-run
aigc-cli mj upscale --task-id task_xxx --index 1 --dry-run
```

## 查看生效配置

打印当前生效的配置（含来源标注）。各命令实际使用的 Provider 由 `defaults.{命令}.provider` 与 `providers` 段共同决定，均在下文中列出。

```bash
aigc-cli --print-config
```

输出前所有密钥均已脱敏：API Key 仅保留后 4 位，`base_url` / `http_proxy` 中的密码与密钥参数以 `REDACTED` 替代。

## 查看版本

```bash
aigc-cli version
# 或
aigc-cli --version
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
| `POST /v1/video/create` | 文生视频 | OpenLux ✅ | [OpenLux 官方文档](https://doc.openlux.ai/en/tutorials/00-intro) |
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
| `GET /v1/model/{author}/{slug}` | 单个模型详情（上下文窗口、定价；注意为单数端点） | OpenRouter ✅ | [OpenRouter Get Model](https://openrouter.ai/docs/api/api-reference/models/get-a-model-by-its-slug) |

各端口的接口规范详细参考来源、Provider 检测机制和策略路由说明见 [api-reference.md](api-reference.md)。
