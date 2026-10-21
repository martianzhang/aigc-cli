# 其他命令

## 检测 AIGC

详见 [docs/guide-detect.md](guide-detect.md)。

## 音乐生成

自然语言生成音乐（APIMart suno/flowmusic 异步、OpenRouter Lyria 同步流式），详见 [guide-music.md](guide-music.md)。

## 联网搜索（search）

`aigc-cli search` 搜索网页并**直接输出实际执行厂商返回的原始 JSON**，不做 title/url 归一化。这样 snippets、score、metadata 等厂商字段都保留下来，便于调试和脚本处理。未配置任何第三方厂商时自动使用内置 DuckDuckGo，免费免 Key：

```bash
aigc-cli search "golang context timeout"
aigc-cli search golang context timeout            # 多个参数自动拼成 query
aigc-cli search "vector database" -n 10           # 请求 10 条（默认 5，上限 20）
aigc-cli search "vector database" -P deepseek     # 指定搜索厂商
aigc-cli search "vector database" -P free         # 只用免费引擎
echo "golang context timeout" | aigc-cli search  # 从 stdin 读取 query
aigc-cli search - < query.txt                     # 显式从 stdin 读取
aigc-cli search --json query.json                 # 命令级 JSON（文件）
aigc-cli search --json '{"query":"golang context","provider":"exa","limit":3}'
aigc-cli search "golang" --inject-json '{"type":"neural"}'
aigc-cli search "vector database" --dry-run       # 打印等价 curl，不发请求
```

| 参数 | 默认 | 说明 |
|---|---|---|
| `-P, --provider` | 空（→ auto） | 搜索厂商：`duckduckgo` / `firecrawl` / `brave` / `doubao` / `deepseek` / `exa`，或策略 `auto` / `free` / `quality` |
| `-n, --limit` | `5` | 请求结果条数，上限 20 |
| `--json` | 空 | 命令级 JSON：文件路径或 inline JSON（**不接受 stdin**），字段：`query`、`provider`、`limit` |
| `--inject-json` | 空 | 厂商原生字段：文件路径或 inline JSON；POST 合并进请求体，GET 变成 query 参数 |
| `--dry-run` | `false` | 打印每个候选厂商的等价 curl（API key 掩码，含注入字段），不调用任何搜索 API |

- **stdin 用来传 query**：`echo "..." | aigc-cli search` 或无参数时自动读取管道/文件；`aigc-cli search -` 显式从 stdin 读取。多行输入会压成一行。`--json` / `--inject-json` 都不读 stdin。

- **`--provider` 指搜索厂商，不是 LLM provider**；优先级：`--provider` > `--json` 的 `provider` > `defaults.search.provider` > `auto`。
- 查询与 limit 的优先级同样是：命令行位置参数 / `--limit` > `--json` > 默认值。
- `--inject-json` 遵循全局约定：对象递归合并、数组追加、标量覆盖。POST 厂商直接合入 JSON body；GET 厂商（Brave）只接受标量或标量数组并转为 query 参数，嵌套对象会报错。
- DuckDuckGo 是 HTML 抓取，不接受 `--inject-json`，会明确报错。
- 输出就是厂商响应体：合法 JSON 会做缩进美化；不是合法 JSON 时原样输出（便于看到错误页）。
- DuckDuckGo 没有厂商 JSON，输出为 `{"urls":["..."]}`。
- 配置了多个厂商时，`auto` 会把带 `free` 标签的厂商排在候选前面，最终按 `weight` 随机选择；要强制只用免费引擎请用 `-P free`，要固定厂商请用 `-P <name>`。
- `--dry-run` 不产生网络请求。`auto` 下会列出所有候选厂商的 curl；最终只会调用其中一个。
- 只搜索、不写知识库。需要“搜索 + 抓取 + 入库”请用 `aigc-cli kb search`；单独抓取 URL 的 `aigc-cli fetch` 仍在 TODO。
- `-v` 对搜索命令不再有额外输出；标准输出始终只有厂商 JSON。

`--json` 输入示例：

```json
{
  "query": "golang context",
  "provider": "exa",
  "limit": 3
}
```

`--dry-run` 示例（带 `--inject-json`）：

```bash
# dry run: no request sent
# query: golang
# strategy: manual
# provider: exa (candidate 1/1)
curl -X POST https://api.exa.ai/search \
  -H "x-api-key: ...3644" \
  -H "Content-Type: application/json" \
  -d '{"numResults":4,"query":"golang","type":"neural"}'
```

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
aigc-cli models --provider openrouter --type image   # 展示架构、参数、能力
aigc-cli models --provider openrouter --type video

# 查询单个模型详情（展示上下文窗口、定价、模态、支持参数）
aigc-cli models --provider openrouter openai/gpt-4o
# 也可用全局 --model/-m 指定（等价于上面的位置参数）
aigc-cli models --provider openrouter --model openai/gpt-4o

# 只打印等价 curl，不真正调用接口（零成本）
aigc-cli models --model gpt-4o --dry-run
aigc-cli models --type image --dry-run

# OpenAI 标准模型列表
aigc-cli models --api-base "https://api.openai.com/v1"
```

> **单模型查询**：`aigc-cli models <模型名>` 或 `aigc-cli models --model <模型名>` 展示单个模型的详情（位置参数优先）。OpenRouter 走其**单数端点** `GET /v1/model/{author}/{slug}`（与列表的复数 `/models` 不同），并**原样输出接口返回的 JSON（仅格式化缩进，不做字段解析）**；其他 Provider 直接拉取 `GET /v1/models` 列表并按模型名过滤（不再探测 `GET /v1/models/{id}`——多数 Provider 并不支持该端点），无需额外配置。

> **`--dry-run`**：只渲染等价的 `curl` 命令（API Key 已脱敏），不发起任何网络请求。列表、单模型详情、市场（`--type`）、定价（`--price`）与 OpenRouter 发现均支持；市场分页只打印第一页，后续页码以注释说明。

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

查询支持余额接口的 Provider：APIMart（及兼容中转）、OpenRouter、DeepSeek、Moonshot（Kimi）。其他 Provider 无公开余额接口，请到其控制台查看。

**默认会查询并展示该 Provider 暴露的每一个余额端点**（例如 APIMart 的 Key 配额 + 账户余额、OpenRouter 的 Key 用量 + 账户信用点）。`token` / `user` 子命令可只取其中一个维度。

```bash
# 查询所有已配置且带 API Key 的 Provider（展示全部端点）
aigc-cli balance

# 查询指定 Provider
aigc-cli balance --provider openrouter

# 只查 API Key 维度 / 只查账户维度
aigc-cli balance token
aigc-cli balance user
```

> **API Key**：未指定 `--provider` 时，会查询所有已配置且带 API Key 的 Provider（Ollama 等本地 Provider 免密钥）。显式指定但未配置密钥的非本地 Provider 会直接报错并给出提示；用 `--provider <name>` 指定已配置密钥的 Provider。

> **OpenRouter**：账户信用点余额（`GET /credits`，与控制台一致）需要 management key；普通推理 Key 只有 Key 维度的用量/额度（`GET /key`）。

> **SiliconFlow**：其 `/user/info` 余额接口已于 2026-08-14 停止服务且暂无替代，因此显示 "not available"。

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

- 配置文件解析规则与其他命令一致：优先 `--config <path>`，否则 `~/.config/aigc-cli/config.yaml`。显式传入的 `--config` 文件必须存在，否则任何命令都会报 `config file not found` 而不是静默忽略；未指定 `--config` 时文件缺失是允许的，此时回退到 `OPENAI_API_KEY` / `OPENAI_BASE_URL` 等环境变量与代码默认值。`get`/`set` 同样直接报错，**绝不自动创建文件**。
- `set` 只替换叶子值。点号路径中缺少父级段落时会报错（`set defaults.chat.allow_tool_override: section not found`），**绝不自动创建段落**。
- 保留已有键的 YAML 类型：原本是 int/bool/float 就继续保持该类型，无法解析的值会被拒绝；原本是字符串的仍是字符串。在已有段落中新建的键，除纯整数或 `true`/`false` 外都按字符串写入。
- 写入是原子的：先把原文件复制为 `<path>.bak`，新内容写入 `<path>.tmp.<pid>`，再用 rename 覆盖目标文件。注释、键顺序、标量引号风格都会保留；空行与个别空格的排版可能被规范化。
- `api_key` / `api_secret`（含 `providers.*`、`web_search.*`）在写入前会用本地主密钥加密（`enc:v1:...`）；此外，任意 `aigc-cli` 命令启动时都会扫描 `config.yaml`，发现明文密钥就自动加密。加密不写明文 `.bak`，其余字段不受影响。
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
| `GET /api/v1/credits` | 账户信用点余额（需 management key） | OpenRouter ✅ | [OpenRouter Credits](https://openrouter.ai/docs/api/api-reference/credits/get-remaining-credits) |
| `GET /api/v1/key` | Key 维度用量/额度（普通 Key 可用） | OpenRouter ✅ | [OpenRouter Key](https://openrouter.ai/docs/api/api-reference/api-keys/get-current-api-key) |
| `GET /user/balance` | 账户余额（多币种） | DeepSeek ✅ | [DeepSeek Docs](https://api-docs.deepseek.com/api/get-user-balance/) |
| `GET /v1/users/me/balance` | 账户余额（人民币） | Moonshot ✅ | [Kimi Docs](https://platform.kimi.com/docs/api/balance) |
| `GET /api/marketplace/models` | 模型列表（免认证） | APIMart ✅ | [APIMart Docs](https://docs.apimart.ai/en) |
| `GET /api/pricing/model` | 模型定价详情（免认证） | APIMart ✅ | [APIMart Docs](https://docs.apimart.ai/en) |
| `GET /api/image2studio.com/public/prompts/search` | 提示词灵感搜索 | 通用 ✅ | [Image2Studio](https://image2studio.com/prompts) |
| `GET /v1/models` | 模型列表 | OpenAI/OpenRouter ✅ | [OpenAI Models](https://platform.openai.com/docs/api-reference/models/list) / [OpenRouter Models](https://openrouter.ai/docs/api/api-reference/models/get-models) |
| `GET /v1/model/{author}/{slug}` | 单个模型详情（上下文窗口、定价；注意为单数端点） | OpenRouter ✅ | [OpenRouter Get Model](https://openrouter.ai/docs/api/api-reference/models/get-a-model-by-its-slug) |

各端口的接口规范详细参考来源、Provider 检测机制和策略路由说明见 [api-reference.md](api-reference.md)。
