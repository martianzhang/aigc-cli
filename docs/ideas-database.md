# ideas 搜索：SQLite 持久化索引方案

## 背景

`aigc-cli ideas` 命令从 `ideas.json`（~61MB，30 万条）中搜索 AI 图片提示词。当前实现每次执行都全量加载 JSON 到内存并重建 BM25 索引，内存峰值超过 1GB，在低内存环境会被 OOM killer 杀死。

## 目标

- 消除全量数据加载，将内存峰值降至 ~10MB + 候选集大小
- 保持现有搜索算法不变（BM25 + n-gram cosine + RRF fusion + title boost）
- 零 CGo 依赖，不破坏交叉编译
- `ideas init` 时构建索引，`ideas.json` 更新时自动重建

## 方案：SQLite + 倒排索引

### 核心思路

SQLite 作为持久化数据存储，在 `ideas init` 阶段：
1. 将 `ideas.json` 导入 SQLite `ideas` 表
2. 构建**倒排索引**（inverted index）存到 `inverted_index` 表

搜索时：
1. `tokenize(query)` 得到查询词列表
2. 倒排索引取交集 → 满足 AND 条件的 `entry_id` 集合
3. 只加载这些候选行（通常几十到几百条）到内存
4. 走现有的 `BuildBM25Index` + `searchIdeas` 排序

### Schema

```sql
-- meta 表：元数据（校验和、统计信息）
CREATE TABLE meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
-- key 示例:
--   'source_hash'    → ideas.json 的 SHA256
--   'total_docs'     → 总文档数
--   'avg_doc_len'    → 平均文档长度（token 数）
--   'schema_version' → schema 版本号

-- ideas 表：完整条目数据
CREATE TABLE ideas (
    id INTEGER PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    title_zh TEXT NOT NULL DEFAULT '',
    prompt TEXT NOT NULL,
    prompt_zh TEXT NOT NULL DEFAULT '',
    image_urls TEXT NOT NULL DEFAULT '[]',
    source_url TEXT NOT NULL DEFAULT '',
    author TEXT NOT NULL DEFAULT '',
    license TEXT NOT NULL DEFAULT '',
    lang TEXT NOT NULL DEFAULT ''
);

-- 倒排索引表：term → [entry_id]
-- WITHOUT ROWID 减少存储开销
CREATE TABLE inverted_index (
    term TEXT NOT NULL,
    entry_id INTEGER NOT NULL,
    tf INTEGER NOT NULL,
    PRIMARY KEY (term, entry_id)
) WITHOUT ROWID;

-- 索引：支持快速取交集 (term1 INTERSECT term2)
CREATE INDEX idx_inverted_entry ON inverted_index(entry_id);
```

### 导入流程（`ideas init`）

```
1. 计算 ideas.json 的 SHA256
2. DB 文件已存在？
   ├─ 读取 meta.source_hash
   ├─ 相同 → 跳过（索引已是最新）
   └─ 不同 → 重建
3. DB 不存在 → 新建
   a. 执行 CREATE TABLE
   b. 遍历 ideas.json 条目，逐批写入 ideas 表
   c. tokenize 每一条，写入 inverted_index 表
   d. 计算 avg_doc_len，写入 meta 表
   e. 写入 source_hash / total_docs / schema_version
```

### 搜索流程

```sql
-- Step 1: 倒排索引取交集，得到候选 entry_id
SELECT entry_id FROM inverted_index WHERE term = ?
INTERSECT
SELECT entry_id FROM inverted_index WHERE term = ?
-- ... 每个 query term 一个 INTERSECT

-- Step 2: 加载候选条目的完整数据
SELECT * FROM ideas WHERE id IN (?, ?, ?, ...)
```

```go
// Step 3: 用现有 BM25 算法在内存中对候选集排序
idx := BuildBM25Index(entries) // 只在候选集上构建
results := searchIdeas(entries, idx, query)
```

BM25 的 IDF 在导入时已建好（存为 `total_docs`），查询时只需从 `inverted_index` 查每个 term 的文档频率（`COUNT(*)`），即可算出 `idf = log(1 + (N - df + 0.5) / (df + 0.5))`。

### 文件变动

| 文件 | 操作 | 说明 |
|---|---|---|
| `go.mod` | 修改 | 新增 `modernc.org/sqlite` 依赖 |
| `internal/ideas/sqlite.go` | **新建** | DB 初始化、导入、倒排索引构建、搜索查询 |
| `internal/ideas/search.go` | 修改 | `SearchText`/`SearchRandom` 改为接受 `*sql.DB` |
| `internal/ideas/bm25.go` | 修改 | 支持从外部传入 IDF，不在全量构建 |
| `cmd/ideas_cache.go` | 修改 | 新增 `ideasDBSavePath()`，DB 路径优先于 JSON |
| `cmd/ideas_init.go` | 修改 | 改为导入 SQLite + 存 SHA256 |
| `cmd/ideas.go` | 修改 | `runIdeas` 改用 SQLite DB |
| `internal/mcp/server.go` | 修改 | MCP handler 中 `SearchText`/`SearchRandom` 调用适配 |
| `internal/ideas/format.go` | 不改 | 输出格式化不变 |
| `internal/ideas/types.go` | 不改 | 数据结构不变 |

### 增量更新

```
ideas init 执行时：
  1. 计算 ideas.json 的 SHA256
  2. 读取 DB 中 meta.source_hash
  3. 相同 → exit 0（无需操作）
  4. 不同 → 删除 DB 文件，重新导入

强制重建：
  aigc-cli ideas init --force   # 忽略 SHA256，强制重建
```

## 性能预期

| 指标 | 当前 | SQLite 方案 |
|---|---|---|
| 冷启动内存 | ~1GB+ | ~10MB |
| 索引构建时间 | ~3s（并行 + 全量） | ~3s（导入时一次） |
| 搜索时间（无关键词随机） | 2-3s | <50ms |
| 搜索时间（有关键词） | 2-3s | <100ms |

## 未解决的问题 / Future work

- `ideas.json` 本身 61MB 的下载问题：当前依赖 `ideas init` 从 GitHub 下载，`source_hash` 的校验只保证索引和 JSON 一致，不保证 JSON 本身是最新版
- 后续可考虑直接更新 ideas 表（增量 upsert），而不是全量重建

## 附录：为什么要用 SQLite 而不是 FTS5

FTS5 的 tokenizer 对 CJK 做单字拆分，而我们现有的 tokenizer 对 CJK 做 2-gram 拆分，且搜索包含 AND filter、n-gram cosine、RRF fusion 等多阶段排序。直接用 FTS5 做检索会导致召回集和质量与现有行为不一致。

因此 SQLite 在此方案中仅作为**持久化存储 + 倒排索引**使用，排序算法保持 100% 不变。
