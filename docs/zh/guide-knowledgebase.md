# 知识库使用指南

知识库让你在本地存储文档和网页内容，支持全文检索、语义搜索（基于 ONNX 嵌入模型），以及 age 加密保险箱。

## 快速开始

```bash
# 1. 初始化（建表、下载模型）
aigc-cli kb init

# 2. 添加内容
aigc-cli kb add README.md                    # 本地文件
aigc-cli kb fetch https://go.dev/doc/         # 网页
aigc-cli kb map https://example.com --dry-run # 发现链接

# 3. 搜索
aigc-cli kb find "如何安装"                   # 关键词 + 语义搜索
aigc-cli kb find "go 1.24" --show             # 显示匹配内容

# 4. 联网搜索 + 自动入库
aigc-cli kb search "rust vs go 2024"

# 5. 查看文档
aigc-cli kb list                              # 列出所有文档
aigc-cli kb show b08049dce61f                 # 读文档全文
```

## 命令

### 初始化与维护

```bash
aigc-cli kb init              # 初始化（幂等）
aigc-cli kb index             # 从 docs/ 目录重建索引
aigc-cli kb prune             # 去重
aigc-cli kb prune --check-urls  # 去重 + 检查 URL 是否 404
aigc-cli kb reset             # 清空全部数据（需确认）
aigc-cli kb reset --force     # 强制清空
```

### 添加内容

```bash
aigc-cli kb add file.md                    # 文本 / Markdown / 代码等
aigc-cli kb add doc.pdf                    # PDF（需配置 pdftotext 加载器）
aigc-cli kb add . --recursive              # 递归添加目录下所有文件
aigc-cli kb fetch https://example.com      # 抓取网页
aigc-cli kb map https://blog.example.com   # 发现页面链接，批量入库
aigc-cli kb map https://example.com --dry-run  # 仅预览，不抓取
aigc-cli kb map https://example.com --limit 5  # 控制数量
```

### 搜索

```bash
aigc-cli kb find "query"                    # 默认搜索（FTS5 + 语义）
aigc-cli kb find "query" --project          # 只搜当前项目
aigc-cli kb find "query" --all              # 搜所有项目
aigc-cli kb find "query" --show             # 显示匹配内容
aigc-cli kb search "query"                  # 联网搜索 + 自动入库
```

### 浏览与管理

```bash
aigc-cli kb list                            # 列出文档
aigc-cli kb list --all                      # 列出所有项目
aigc-cli kb show <doc-id>                   # 查看文档全文
aigc-cli kb rm <doc-id>                     # 删除
```

### 保险箱（加密）

```bash
aigc-cli kb add secret.docx --vault          # 加密存储
aigc-cli kb fetch https://... --vault        # 加密抓取
aigc-cli kb list --vault                     # 列出保险箱
aigc-cli kb show <id> --vault                # 解密查看
aigc-cli kb vault export backup.tar.gz       # 导出（含私钥）
aigc-cli kb vault import backup.tar.gz       # 导入
```

> 保险箱使用 CLI 的**本地加密主密钥**（age identity）。它在首次运行任意 `aigc-cli` 命令时自动生成并存入系统钥匙串，**无需 `kb init`**。无头/CI 环境请设置 `AIGC_CLI_MASTER_KEY`（见安装文档）。

## 保险箱 vs 知识库

| | 知识库 | 保险箱 |
|---|---|---|
| 内容存储 | Markdown 明文 | age 加密 `.age` 文件 |
| SQLite 数据 | 全文索引 + 向量 + 元数据 | 仅向量（不可还原原文） |
| 搜索方式 | FTS5 全文检索 + 向量语义 | 仅向量搜索 |
| 解密 | 不需要 | 系统钥匙串 |
| 安全性 | 全盘加密保护 | age 加密 + 钥匙串 |

保险箱中的内容仅以向量形式存在于 SQLite，无法还原为原文。实际内容加密存储在 `~/.config/aigc-cli/vault/docs/`。

## 项目隔离

在 git 仓库内运行时，文档自动归属到当前项目。项目标识取自 git remote origin URL（如 `github.com/org/repo`）。

```bash
cd /workspace/myapp
aigc-cli kb add README.md            # 自动归到 myapp 项目
aigc-cli kb find "api" --project     # 只搜当前项目
aigc-cli kb find "api" --all         # 搜全部项目
aigc-cli kb list                     # 默认只列当前项目
```

## 搜索质量

搜索分两种模式：

1. **FTS5 关键词搜索**——精确匹配，任何语言都能搜
2. **向量搜索**——语义理解；embedding 后端可插拔：内置 ONNX（multilingual-e5-small，384 维）、本地 Ollama，或任意 OpenAI 兼容的 `/v1/embeddings` 厂商

两者并行执行，结果融合排序。向量结果默认最低相似度 0.8，低于此的自动过滤。可在配置中调整：

```yaml
defaults:
  knowledgebase:
    min_score: 0.5    # 阈值越低召回越多（默认 0.8）
    embedding_provider: ollama        # 留空 / local / onnx / hash，或 config.providers 里的名字
    embedding_model: embeddinggemma-2 # 模型 id（命名 provider 必填）
```

首次 `kb init` 会自动下载内置 embedding 模型（~130MB）。有 CGO 时启用 ONNX 推理，无 CGO 时降级为 HashEmbedder；配置了 `embedding_provider` 时改用该后端（见下）。

## Embedding 后端

语义检索的**入库与查询必须用同一个后端**，由 `defaults.knowledgebase.embedding_provider` + `embedding_model` 选择：

| `embedding_provider` | 后端 |
|---|---|
| 留空 / `local` / `onnx` | 内置 ONNX E5；ONNX 不可用时降级为纯 Go Hash embedder |
| `hash` | 纯 Go n-gram 哈希（无需模型，质量低） |
| `config.providers` 里的任意名字 | OpenAI 兼容的 `/v1/embeddings`——本地 Ollama 或在线厂商 |

命名 provider 提供 `base_url` / `api_key` / `http_proxy`；`embedding_model` 填厂商模型 id（Ollama 用 `embeddinggemma-2`，OpenAI 用 `text-embedding-3-small`）。一篇文档的所有 chunk 在一次批量请求里完成 embedding。

> 更换后端会改变向量维度，旧向量不再匹配。`kb` 检测到不一致会告警——执行 `aigc-cli kb reset` 并重新添加文档以重建索引。

## 外部加载器

对于不支持的文件格式，可配置外部命令自动转换：

```yaml
defaults:
  knowledgebase:
    loaders:
      .pdf: "pdftotext $1 -"
      .docx: "pandoc --to markdown --wrap=none $1"
```

命令中的 `$1` 会被替换为文件路径，stdout 输出作为文档内容。

## Web 搜索

`kb search` 使用配置的 web_search 提供商：

```yaml
web_search:
  duckduckgo:
    type: duckduckgo    # 零配置
  brave:
    type: brave
    api_key: "BSA-xxx"  # Brave Search API
  firecrawl:
    type: firecrawl
    api_key: "fc-xxx"
  doubao:
    type: doubao
    api_key: "your-api-key"   # 火山引擎豆包搜索（Custom版）
```

配置多个 provider 时会自动根据配额和策略进行 fallback。

> **豆包搜索**：火山引擎联网搜索（Custom版），0.020元/次（按量后付费），每月 500 次免费额度。
> API Key 获取：[联网搜索控制台](https://console.volcengine.com/search-infinity/api-key?tab=post_paid)。
> 认证方式：`Authorization: Bearer <API_KEY>`。

## 存储位置

```
~/.config/aigc-cli/
├── knowledge/
│   ├── knowledge.db              # SQLite（FTS5 + 向量 + 元数据）
│   └── docs/                     # 明文 Markdown 文件
│       ├── global/               # 全局文档
│       └── github.com_org_repo/  # 项目文档
├── vault/
│   ├── metadata.json
│   └── docs/<sha1>.age           # age 加密文件（不可直接读）
└── models/
    └── e5-small-multilingual/    # ONNX embedding 模型
```

## MCP / Chat 工具

启动 MCP Server 后自动注册 6 个工具：

| 工具 | 作用 |
|---|---|
| `kb_find` | 搜索知识库 |
| `kb_search` | 联网搜索 + 自动入库 |
| `kb_add` | 添加本地文件 |
| `kb_fetch` | 抓取 URL |
| `kb_list` | 列出文档 |
| `kb_show` | 读取文档全文 |

Chat 模式同样支持以上 6 个工具，Agent 可在对话中直接操作知识库。
