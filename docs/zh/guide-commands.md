# 其他命令

## 检测 AIGC

详见 [docs/guide-detect.md](guide-detect.md)。

## 音乐生成

自然语言生成音乐（APIMart suno/flowmusic 异步、OpenRouter Lyria 同步流式），详见 [guide-music.md](guide-music.md)。

## 联网搜索（search）

`aigc-cli search` 搜索网页或素材站（Pixabay / Pexels）并**直接输出实际执行厂商返回的原始 JSON**，不做 title/url 归一化。这样 snippets、score、metadata、图片/视频下载链接等厂商字段都保留下来，便于调试和脚本处理。未配置任何第三方厂商时自动使用内置 DuckDuckGo，免费免 Key：

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
aigc-cli search --list-providers
aigc-cli search "vector database" --dry-run       # 打印等价 curl，不发请求
```

素材搜索（Pixabay / Pexels）用 `--type` 选择图片/视频端点，用 `--inject-json` 传厂商自己的筛选参数；查询词、条数和 API key 由 CLI 持有，不会被注入覆盖。Pixabay 支持 `image`（全部图片）/ `photo` / `illustration` / `vector` / `video`；Pexels 只有照片和视频，因此只支持 `image` / `video`（`image` 就是照片）。

```bash
# Pixabay 图片（photo / illustration / vector / image）
aigc-cli search -P pixabay "雨夜 车窗" --type photo -n 12 \
  --inject-json '{"orientation":"horizontal","min_width":1920,"safesearch":"true"}'

# Pixabay 视频
aigc-cli search -P pixabay "雨夜 车窗" --type video -n 12 \
  --inject-json '{"min_width":1920}'

# Pexels 视频
aigc-cli search -P pexels "rain window night" --type video -n 12 \
  --inject-json '{"orientation":"landscape","size":"large"}'
```

输出是厂商原始 JSON，里面的 `pageURL` / `url`、预览图和下载链接可直接交给 `aigc-cli fetch` 下载。素材的授权仍需按来源逐条确认（Pixabay Content License / Pexels License 都允许免费使用，但有各自限制）。

音频素材（SFXMint / Freesound）同样只透传厂商 JSON，不加额外筛选参数，过滤条件走 `--inject-json`：

```bash
# SFXMint：免 API key，全部 CC0，结果 URL 就是 WAV 直链
aigc-cli search -P sfxmint "door slam" -n 5
aigc-cli search -P sfxmint "ui click" -n 5 --inject-json '{"max_duration_ms":1000,"loop":false}'

# Freesound：需要 api_key；用厂商的 filter 只要 CC0、时长 0-3 秒
aigc-cli search -P freesound "door slam" -n 5 \
  --inject-json '{"filter":"license:\"Creative Commons 0\" duration:[0 TO 3]"}'

# 下载搜到的音效
aigc-cli fetch "https://sfxmint.com/dl/metal-door-slam-02.wav" -o sfx/
```

> Freesound 的原始 WAV 下载需要 OAuth2，CLI 因此把结果 URL 指向公开预览 MP3（`preview-hq-mp3`）；要原档请到声音页面手动下载。SFXMint 的结果 URL 直接是 WAV/MP3，可直接 `fetch`。

| 参数 | 默认 | 说明 |
|---|---|---|
| `-P, --provider` | 空（→ `duckduckgo`） | 搜索厂商名：`duckduckgo` / `firecrawl` / `brave` / `doubao` / `deepseek` / `exa` / `wsa` / `pixabay` / `pexels` / `sfxmint` / `freesound` |
| `--type` | 空 | 素材端点：`image` / `photo` / `illustration` / `vector` / `video`；网页搜索厂商忽略 |
| `-n, --limit` | `5` | 请求结果条数，上限 20 |
| `--json` | 空 | 命令级 JSON：文件路径或 inline JSON（**不接受 stdin**），字段：`query`、`provider`、`type`、`limit` |
| `--inject-json` | 空 | 厂商原生字段：文件路径或 inline JSON；POST 合并进请求体，GET 变成 query 参数；查询词、条数、API key 保留由 CLI 控制 |
| `--refresh` | `false` | 忽略厂商响应缓存（Pixabay 按 API 要求缓存 24h） |
| `--dry-run` | `false` | 打印选中厂商的等价 curl（API key 掩码，含注入字段），不调用任何搜索 API |
| `--list-providers` | `false` | 列出已配置厂商、search/fetch/media 能力与可用状态（含不可用原因） |

- **stdin 用来传 query**：`echo "..." | aigc-cli search` 或无参数时自动读取管道/文件；`aigc-cli search -` 显式从 stdin 读取。多行输入会压成一行。`--json` / `--inject-json` 都不读 stdin。

- **`--provider` 指搜索厂商，不是 LLM provider**；优先级：`--provider` > `--json` 的 `provider` > `defaults.search.provider` > `duckduckgo`。
- 查询、`--type` 与 limit 的优先级同样是：命令行位置参数 / `--type` / `--limit` > `--json` > 默认值。
- `--inject-json` 遵循全局约定：对象递归合并、数组追加、标量覆盖。POST 厂商直接合入 JSON body；GET 厂商（Brave / Pixabay / Pexels）只接受标量或标量数组并转为 query 参数，嵌套对象会报错。素材搜索的 query、`per_page`、API key 由 CLI 保留，不会被注入覆盖。
- DuckDuckGo 是 HTML 抓取，不接受 `--inject-json`，会明确报错。
- 输出就是厂商响应体：合法 JSON 会做缩进美化；不是合法 JSON 时原样输出（便于看到错误页）。素材搜索的 JSON 包含预览图和下载链接，可直接用 `aigc-cli fetch` 保存。
- DuckDuckGo 没有厂商 JSON，输出为 `{"urls":["..."]}`。
- 每次只调用一个厂商；厂商失败直接报错，不会自动切换或重试其他厂商。
- Pixabay 的响应会按 API 要求缓存 24 小时；不要写循环做批量抓取。
- 配置了但构造失败的厂商（例如缺少 `api_key`、`type` 拼错）会显示为 `configured but unavailable`，而不是从 available 列表静默消失；`--list-providers` 可查看具体原因。
- `--dry-run` 不产生网络请求，只打印选中厂商的 curl。
- 只搜索、不写知识库。需要“搜索 + 抓取 + 入库”请用 `aigc-cli kb search`；单独抓取 URL 用 `aigc-cli fetch`。
- `-v` 对搜索命令不再有额外输出；标准输出始终只有厂商 JSON。
- 所有搜索请求都走全局代理：配置文件 `http_proxy` 或环境变量 `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY`。

`--json` 输入示例：

```json
{
  "query": "rain window",
  "provider": "pexels",
  "type": "video",
  "limit": 6
}
```

`--dry-run` 示例（带 `--inject-json`）：

```bash
# dry run: no request sent
# query: golang
# strategy: manual
# provider: exa
curl -X POST https://api.exa.ai/search \
  -H "x-api-key: ...3644" \
  -H "Content-Type: application/json" \
  -d '{"numResults":4,"query":"golang","type":"neural"}'
```

## 网页抓取（fetch）

`aigc-cli fetch` 抓取一个 URL 或转换一个本地 HTML/文本文件：文本正文转成 Markdown 输出到 stdout；图片/视频/音频等媒体响应保存到 `-o` 目录并打印保存路径。默认使用内置免费抓取；不落盘（文本）、不入库：

```bash
aigc-cli fetch https://go.dev/doc/effective_go
aigc-cli fetch https://go.dev/doc/effective_go > effective_go.md
aigc-cli fetch ./page.html > page.md
aigc-cli fetch /tmp/report.html --max-chars 20000
aigc-cli fetch https://cdn.example/clip.mp4 -o stock/ --name rain-01
aigc-cli fetch https://cdn.example/photo.jpg -o stock/ --name still-01
aigc-cli fetch https://cdn.example/clip.mp4 -o stock/ --name rain-01.mp4 --force
aigc-cli fetch https://example.com/article --html > page.html
aigc-cli fetch https://example.com/private -H "Authorization: Bearer $TOKEN"
aigc-cli fetch https://example.com/private -b cookies.txt
aigc-cli fetch https://example.com/private -b a.txt -b b.txt -c cookies.txt
aigc-cli fetch https://example.com/private -b "session=abc; theme=dark"
aigc-cli fetch https://example.com/article --max-chars 20000
aigc-cli fetch https://example.com/app --provider deepseek
aigc-cli fetch https://example.com/article --dry-run
aigc-cli fetch --list-providers
```

| 参数 | 默认 | 说明 |
|---|---|---|
| `-P, --provider` | 空 = 内置抓取 | 抓取厂商名：`deepseek` / `exa` |
| `-H, --header` | 无 | 可重复，`Name: value`；`Name:` 删除默认 header（仅内置抓取） |
| `-b, --cookie` | 无 | 可重复；含 `=` 按 cookie 字符串解析，否则按 Netscape cookie 文件读取（仅内置抓取） |
| `-c, --cookie-jar` | 无 | 抓取结束后把 cookie 写回 Netscape 格式文件（仅内置抓取） |
| `--name` | 无 | 媒体响应的文件名；不带扩展名时自动补检测到的扩展名，带扩展名则原样使用；不能含路径分隔符 |
| `--force` | `false` | 覆盖已存在的媒体文件；默认拒绝覆盖 |
| `--max-chars` | `0`（不截断） | 按字符（rune）截断文本输出 |
| `--html` | `false` | 输出原始 HTML / 原始文本，不做 readability/Markdown 转换（仅内置抓取） |
| `--dry-run` | `false` | 打印等价 curl，不发请求 |
| `--list-providers` | `false` | 列出已配置厂商、search/fetch 能力与可用状态（含不可用原因） |
| `-v` / `--timeout` | 全局 | `-v` 输出标题 / 最终 URL / 类型 / 大小；媒体响应还输出 SHA256 与保存路径；`--timeout` 控制内置抓取超时（大文件可用 `--timeout 300`） |

- 默认内置抓取：HTML 经 readability 提取正文并转 Markdown；`text/plain`、JSON 等文本原样输出；图片/视频/音频/PDF 等二进制媒体按 `Content-Type` 保存到 `-o` 目录（默认当前目录），stdout 只打印保存路径。
- 媒体下载在终端（stderr）显示单行进度：已下载 / 总量、百分比、速度、ETA；完成后无论是否 TTY 都会输出 `downloaded <size> in <time> (<speed>)`。stdout 始终只打印保存路径，便于脚本处理。
- 媒体文件名：不给 `--name` 时优先用 URL basename；`--name rain-01` 自动补 `.mp4` / `.jpg` 等扩展名；`--name rain-01.mp4` 原样使用。扩展名以响应 `Content-Type` 为准，URL 后缀误导时会纠正；未知类型回退 `.bin`。
- 媒体文件已存在时默认报错并且不覆盖；显式加 `--force` 才覆盖。`--name` 只是文件名，不能含 `/` 或 `\`；目录用 `-o`。
- 媒体自动保存只走内置抓取；`--provider` 与 `--name` / `--force` 不能同时使用，本地文件也不适用。
- 参数会先做本地文件存在性检测：相对路径、绝对路径、含空格路径或 `file://` 只要文件存在，就直接转换、不联网；`--provider` / `-H` / `-b` / `-c` / `--name` / `--force` 对本地文件不适用。
- 默认使用浏览器 User-Agent，减少站点返回 JS 跳转/反爬空页；如果正文仍为空，会明确报错而不是输出空白。
- 文章页优先使用 readability 提取正文；如果 readability 只提取到很短的页脚/导航碎片，会回退整页转换，并移除 script/style/footer，保留导航等有用内容。
- 输出会清理 `data:` URI（例如 base64 图片）和 `javascript:` 链接，只保留文本/正常链接，避免把大段 base64 塞进正文。
- **默认永远使用内置抓取，不读取 `defaults.search.provider`**；只有显式 `-P/--provider` 才会走厂商，避免静默计费。
- `-H` 在默认 header（`User-Agent` / `Accept` / `Accept-Language`）之后应用，可覆盖或删除；拒绝 CR/LF 注入。
- `-b` 可重复：含 `=` 视为 `name=value; ...`，否则视为 Netscape cookie 文件；多个来源合并，后出现的同 domain/path/name 覆盖前者。
- Netscape 文件支持标准 7 字段、`#HttpOnly_`、`secure`、`expires`、`domain`、`path`；过期 cookie 自动忽略。
- `-H "Cookie: ..."` 优先于 `-b`；`-H "Cookie:"` 清空 cookie。
- `-c` 在抓取结束后写回 Netscape 格式（含响应 `Set-Cookie`），可用于“先登录再抓取”的多步流程；文件原子写入、权限 `0600`。
- `--provider` 走第三方抓取 API（DeepSeek Fetch / Exa Contents），**可能计费**；第三方请求在厂商侧执行，因此 `-H` / `-b` / `-c` / `--name` / `--force` 与 `--provider` 不能同时使用。
- `--html` 输出原始 HTML（非 HTML 的文本/本地文本则原样输出），跳过 readability/Markdown；仅对文本响应有效，媒体响应仍保存文件；与 `--provider` 互斥。
- 如果指定厂商不支持 fetch（如 `brave` / `duckduckgo`），会自动回退内置抓取并在 stderr 提示；回退时 `-H` / `-b` / `-c` 仍然生效。未知厂商名仍会报错。
- 只抓取、不入库。需要“抓取 + 入库”请用 `aigc-cli kb fetch`；需要搜索请用 `aigc-cli search` / `aigc-cli kb search`。
- `--output-json` 暂未实现。
- 内置抓取与第三方抓取都走全局代理：配置文件 `http_proxy` 或环境变量 `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY`。

`--dry-run` 示例：

```bash
# dry run: no request sent
# provider: built-in
curl -X GET https://example.com/article \
  -H "User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36" \
  -H "Accept: text/html,application/xhtml+xml,application/xml;q=0.9,text/plain;q=0.8,*/*;q=0.5" \
  -H "Accept-Language: zh-CN,zh;q=0.9,en;q=0.8"
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

`search` / `fetch` 的 `--provider` 也接入了动态补全：按当前 `web_search.*` 配置提示可用厂商；`fetch` 只提示实现抓取能力的厂商。

## 查询模型列表

支持三个数据源，自动根据 API 地址选择：

| base_url | `--type` 行为 | `--price` 行为 | 无参数行为 |
|---|---|---|---|
| APIMart 域名 | `GET /api/marketplace/models?type=...`（image / video / chat） | APIMart 定价 API | `GET /v1/models` |
| OpenRouter 域名 | `GET /v1/models?output_modalities=<类型>`（支持全部输出模态）；image / video 改走 `GET /v1/images\|videos/models` 以展示分辨率、宽高比与参数范围 | 单模型记录 `GET /v1/model/{author}/{slug}`（含定价） | `GET /v1/models` |
| 其他（OpenAI 等） | `GET /api/marketplace/models`（APIMart 市场；仅 image / video / chat） | APIMart 定价 API | `GET /v1/models` |

```bash
# 自动选择数据源
aigc-cli models

# 列出已配置的 Provider（名称 / 类型 / Host / 是否配置 Key；不发起网络请求）
aigc-cli models --list-providers

# APIMart 市场（按类型筛选）
aigc-cli models --type image
aigc-cli models --type video
aigc-cli models --type chat

# APIMart 特定模型定价
aigc-cli models --price gpt-image-2-official

# OpenRouter 模型发现（免认证，无需 API Key）
# 支持全部输出模态：text/chat、image、video、audio、speech(tts)、
# transcription(stt)、embeddings、rerank、decisions、all
aigc-cli models --provider openrouter --type image          # 分辨率、宽高比、参数范围
aigc-cli models --provider openrouter --type video
aigc-cli models --provider openrouter --type speech         # TTS 模型 + 可用音色 + 到期时间
aigc-cli models --provider openrouter --type transcription  # 语音转文字
aigc-cli models --provider openrouter --type embeddings
aigc-cli models --provider openrouter --type rerank
aigc-cli models --provider openrouter --type decisions
aigc-cli models --provider openrouter --type all            # 全部模型，附输出模态标签
aigc-cli models --provider openrouter --type tts            # 别名：tts → speech

# 模糊匹配：只列出 id 或名称包含该文本的模型（不区分大小写）
aigc-cli models --like recraft --type image   # 配合 --type 时显示尺寸能力
aigc-cli models --like seedream               # 不带 --type：跨全部模态搜索

# 带价格列（OpenRouter 按 token 计价，展示为 $/M tokens；:free 变体标注 (free)）
aigc-cli models --provider openrouter --type speech --price
aigc-cli models --provider openrouter --price               # 默认 text 列表 + 价格列

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

> **OpenRouter 类型别名**：`chat`/`llm` → `text`，`music` → `audio`，`tts` → `speech`，`stt`/`asr` → `transcription`，`embedding`/`embed` → `embeddings`，`reranking` → `rerank`，`decision` → `decisions`。类型也可以作为位置参数（如 `aigc-cli models -P openrouter speech`）。未知类型会直接报错并列出全部可用值；APIMart 市场只接受 `image` / `video` / `chat`。

> **默认 Provider**：未指定 `-P/--provider`、`--api-base`、`--api-key` 时，`models` 默认使用配置中的 OpenRouter Provider（模型 id 形如 `author/slug`，尺寸元数据最全）；未配置 OpenRouter 时才回落到全局 `base_url`。

> **`--like` 模糊匹配**：按 id 或展示名称做不区分大小写的子串匹配。不带 `--type` 时会跨全部模态搜索（OpenRouter 走 `output_modalities=all`）；配合 `--type image` 时每条结果都会带上 `aspect_ratio` / `resolution` 等能力，是查某个模型尺寸写法最快的方式。

> **单模型查询**：`aigc-cli models <模型名>` 或 `aigc-cli models --model <模型名>` 展示单个模型的详情（位置参数优先）。OpenRouter 走其**单数端点** `GET /v1/model/{author}/{slug}`（与列表的复数 `/models` 不同），并**原样输出接口返回的 JSON（仅格式化缩进，不做字段解析）**；其他 Provider 直接拉取 `GET /v1/models` 列表并按模型名过滤（不再探测 `GET /v1/models/{id}`——多数 Provider 并不支持该端点），无需额外配置。若该模型输出图片（`output_modalities` 含 `image`），CLI 会再查一次 `GET /v1/images/models`，把尺寸能力（`aspect_ratio`、`resolution` 等）和可直接使用的 `--size` 写法打印到 **stderr**，stdout 的 JSON 保持原样。

> **`--dry-run`**：只渲染等价的 `curl` 命令（API Key 已脱敏），不发起任何网络请求。列表、单模型详情、市场（`--type`）、定价（`--price`）与 OpenRouter 发现均支持；市场分页只打印第一页，后续页码以注释说明。

> **API Key**：无参数调用（或查询单个模型，如 `aigc-cli models gpt-4o`）会请求 `/v1/models`，需要 API Key。未指定 `--provider` 且全局未配置 `api_key` / `OPENAI_API_KEY` 时，命令会先打印配置的候选 Provider 列表（名称 / 类型 / Host / 是否配置 Key），再提示用 `--provider <name>` 选择；也可随时用 `--list-providers` 单独查看该列表（不发起网络请求、不需要 Key）。`--type`（类型发现，含 OpenRouter 的全部输出模态）与不带模型名的 `--price`（市场列表 + 价格列）免认证，无需 API Key；`--price <模型名>` 在 APIMart 走公开定价 API（免认证），在 OpenRouter 走 `GET /v1/model/{author}/{slug}`（需要 Key）。

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

查询支持余额接口的 Provider：APIMart（及兼容中转）、OpenRouter、DeepSeek、Moonshot（Kimi）、SoMark（按页计的解析额度）。其他 Provider 无公开余额接口，请到其控制台查看。

**默认会查询并展示该 Provider 暴露的每一个余额端点**（例如 APIMart 的 Key 配额 + 账户余额、OpenRouter 的 Key 用量 + 账户信用点）。`token` / `user` 子命令可只取其中一个维度。

```bash
# 查询所有已配置且带 API Key 的 Provider（展示全部端点）
aigc-cli balance

# 查询指定 Provider
aigc-cli balance --provider openrouter

# 只查 API Key 维度 / 只查账户维度
aigc-cli balance token
aigc-cli balance user

# SoMark / OXR：剩余解析页数（付费页 + 今日/本月免费页）
aigc-cli balance -P somark
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

## 错误提示

参数错误不再打印整页 usage，而是给出可直接照做的短提示：

```bash
$ aigc-cli image --modle "a cat"
Error: unknown flag: --modle
Did you mean --mode, --model?
Run 'aigc-cli image --help' for usage and examples.

$ aigc-cli image --n abc
Error: invalid argument "abc" for "--n" flag: strconv.ParseInt: parsing "abc": invalid syntax
Invalid value "abc" for --n: expected an integer, such as 1.
Run 'aigc-cli image --help' for usage and examples.
```

- 长参数拼错时，按编辑距离（相邻字母颠倒算一次编辑）提示最多 3 个最接近的参数；未知短参数会列出当前命令实际可用的短参数。
- 类型错误按参数类型给出示例：整数、浮点数、布尔值、时长、可重复参数。
- 提示始终指向出错命令自己的 `--help`，不会退回根命令。
- 运行时错误（API、网络、任务失败）保持简短，不会被当作参数错误追加提示。
- 下载与保存的生成结果（图片、视频、音频、base64 解码输入）先写入目标目录的临时文件，成功后再原子重命名；下载或写入失败时，原路径上的旧文件保持不变。

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
| `GET /v1/models?output_modalities=` | 按输出模态发现模型（text / image / video / audio / speech / transcription / embeddings / rerank / decisions / all） | OpenRouter ✅ | [OpenRouter Models](https://openrouter.ai/docs/guides/overview/models) |
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
| `POST /usage` | 剩余解析页数（付费 / 今日 / 本月免费） | SoMark ✅（免费 OXR 不支持） | [SoMark Usage](https://docs.somark.ai/api-reference/endpoint/usage-query) |
| `GET /api/marketplace/models` | 模型列表（免认证） | APIMart ✅ | [APIMart Docs](https://docs.apimart.ai/en) |
| `GET /api/pricing/model` | 模型定价详情（免认证） | APIMart ✅ | [APIMart Docs](https://docs.apimart.ai/en) |
| `GET /api/image2studio.com/public/prompts/search` | 提示词灵感搜索 | 通用 ✅ | [Image2Studio](https://image2studio.com/prompts) |
| `GET /v1/models` | 模型列表 | OpenAI/OpenRouter ✅ | [OpenAI Models](https://platform.openai.com/docs/api-reference/models/list) / [OpenRouter Models](https://openrouter.ai/docs/api/api-reference/models/get-models) |
| `GET /v1/model/{author}/{slug}` | 单个模型详情（上下文窗口、定价；注意为单数端点） | OpenRouter ✅ | [OpenRouter Get Model](https://openrouter.ai/docs/api/api-reference/models/get-a-model-by-its-slug) |

各端口的接口规范详细参考来源、Provider 检测机制和策略路由说明见 [api-reference.md](api-reference.md)。
