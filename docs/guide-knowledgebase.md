# 知识库使用指南

知识库让你在本地存储文档和网页内容，支持全文检索、语义搜索（基于 ONNX 嵌入模型），以及 age 加密保险箱。

## 快速开始

```bash
# 1. 初始化（建表、下载模型、生成加密密钥）
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
2. **ONNX 向量搜索**——基于 `multilingual-e5-small` 模型（384 维），理解语义

两者并行执行，结果融合排序。向量结果默认最低相似度 0.8，低于此的自动过滤。可在配置中调整：

```yaml
defaults:
  knowledgebase:
    min_score: 0.5    # 降低阈值，召回更多结果
    # min_score: 0    # 关闭过滤
```

首次 `kb init` 会自动下载 embedding 模型（~130MB）。有 CGO 时启用 ONNX 推理，无 CGO 时自动降级为 HashEmbedder。

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
```

配置多个 provider 时会自动根据配额和策略进行 fallback。

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
