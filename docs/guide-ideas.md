# 提示词灵感搜索

从本地的 `ideas.json` 文件中搜索 AI 图片生成提示词，找到高质量的风格参考和提示词示例。

无需 API Key。数据来自开源社区整理的优质提示词库，数据文件存储在 **`~/.config/apimart/ideas.json`**（默认位置）。

## 数据准备

### 首次初始化（推荐）

```bash
# 自动下载 ideas.json 并建立搜索索引缓存
apimart-cli ideas init
```

此命令会从 GitHub 下载约 26K 条提示词数据到 `~/.config/apimart/ideas.json`，并在启用缓存时预构建搜索索引。

### 通过 Makefile（开发者用）

```bash
# 从本地源文件生成（需要先下载源数据）
make ideas-data
```

### 搜索索引缓存（默认开启）

缓存默认开启，无需配置。首次搜索自动建立索引并缓存到 `~/.config/apimart/ideas.index`，后续搜索跳过索引构建，启动速度提升 **50-300ms**。

缓存与数据一致性通过 **SHA256 校验**保证：当 `ideas.json` 内容变更（新增/修改条目）时，旧缓存自动失效并重建，无需手动干预。

如需自定义路径或关闭缓存，在 `~/.config/apimart/config.yaml` 中配置：

```yaml
ideas:
  cache_enabled: false                            # 关闭缓存
  data_path: "~/.config/apimart/ideas.json"       # 自定义数据路径
  index_path: "~/.config/apimart/ideas.index"     # 自定义缓存路径
```

数据来源：[NeXra-AI/awesome-ai-image-prompts](https://github.com/NeXra-AI/awesome-ai-image-prompts)（Apache 2.0）及 YouMind 社区。

## 基本用法

```bash
# 搜索提示词（默认 limit=8）
apimart-cli ideas "cinematic portrait"

# 多要一些结果
apimart-cli ideas "luxury perfume" --limit 10

# 随机抽取（搭配关键词）
apimart-cli ideas "portrait" --random

# 随机灵感：不提供关键词，从全量数据中随机返回
apimart-cli ideas --random
apimart-cli ideas --random --limit 1    # 只随机显示一个

# 从 stdin 读取关键词
echo "cyberpunk city" | apimart-cli ideas
```

## 输出格式

默认输出 Markdown，直接打印到终端。每条结果包含标题、参考图、完整提示词和元信息。

```bash
# Markdown 输出（默认），自由重定向到文件
apimart-cli ideas "cat" > my-ideas.md

# JSON 输出，方便用 jq 做二次过滤
apimart-cli ideas "portrait" --json | jq '.results[].prompt'

# 搜索 → jq 提取 prompt → 生成图片
apimart-cli ideas "cat" --json \
  | jq -r '.results[0].prompt' \
  | apimart-cli image --model gpt-image-2 --prompt -
```

### JSON 输出示例

```json
{
  "total": 42,
  "results": [
    {
      "title": "CCD flash beauty portrait template",
      "prompt": "A hyper-photorealistic shot...",
      "image_urls": ["https://raw.githubusercontent.com/..."],
      "source_url": "https://x.com/...",
      "author": "AIwithAliya",
      "license": "Apache 2.0",
      "lang": "en"
    }
  ]
}
```

## 参数

| 参数 | 短参 | 说明 |
|---|---|---|
| `keywords` | | 搜索关键词（位置参数，也从 stdin 读取） |
| `--limit` | `-l` | 返回 N 条结果，默认 8 |
| `--random` | | 从全量结果中随机抽取；不提供关键词时单独使用则从全部数据中随机返回 |
| `--json` | | 输出 JSON 格式（默认 Markdown） |
| `--save` | | 下载参考图片到本地目录 |
| `--output` | | 输出目录（仅 `--save` 时生效，图片存到 `{output}/ideas/images/`） |

## 图片保存

`--save` 参数将参考图片下载到本地，保存在 `{output_dir}/ideas/images/` 目录下：

```bash
apimart-cli ideas "product photography" --save
apimart-cli ideas "cat" --save --output ./my-ideas
```

## 常用搜索词示例

| 搜索词 | 场景 |
|---|---|
| `cinematic portrait` | 电影感人像 |
| `product photography` | 产品摄影 |
| `luxury perfume` | 奢侈品广告 |
| `cyberpunk city` | 赛博朋克城市 |
| `anime character` | 动漫角色 |
| `food photography` | 美食摄影 |
| `fashion editorial` | 时尚大片 |
| `电商` | 中文电商场景 |
| `水墨` | 中国风 |

## 数据来源与版权

本工具的提示词数据来源于开源社区的贡献，衷心感谢以下项目：

- **[NeXra-AI/awesome-ai-image-prompts](https://github.com/NeXra-AI/awesome-ai-image-prompts)** — 955 条精选提示词，Apache 2.0 协议
- **[YouMind](https://youmind.com)** — 11,000+ 提示词灵感

提示词原文和参考图片版权归原作者所有，本工具仅作灵感参考和学习用途。

**如果您是原作者，不希望您的作品出现在本数据集中**，请提交 [issue](https://github.com/martianzhang/apimart-cli/issues) 或联系项目维护者，我们会在收到通知后尽快移除相关内容。
