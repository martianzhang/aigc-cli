# 提示词灵感搜索

从 Image2Studio 的公开提示词库搜索 AI 图片生成灵感，找到高质量的风格参考和提示词示例。

无需 API Key，免费使用。数据来源为 Image2Studio 公开 API。

## 基本用法

```bash
# 搜索提示词（默认 limit=8）
apimart-cli ideas "cinematic portrait"

# 多要一些结果（limit 模式，1-based）
apimart-cli ideas "luxury perfume" --limit 10

# 翻页浏览（page 模式，1-based，每页默认 8 条）
apimart-cli ideas "portrait" --page 2

# 翻页 + 自定义每页数量
apimart-cli ideas "cat" --page 3 --page-size 20

# 从 stdin 读取关键词
echo "cyberpunk city" | apimart-cli ideas
```

## 输出格式

默认输出 Markdown，直接打印到终端。每条结果包含标题、描述、参考图、完整提示词和元信息。

```bash
# Markdown 输出（默认），用户自由重定向到文件
apimart-cli ideas "cat" > my-ideas.md

# JSON 输出，方便用 jq 做二次过滤
apimart-cli ideas "portrait" --json | jq '.results[].prompt'
```

### JSON 输出示例

```json
{
  "total": 42,
  "results": [
    {
      "title": "百叶窗光影下的电影感肖像",
      "description": "生成一张具有戏剧性光影效果的逼真肖像...",
      "prompt": "A photorealistic, cinematic portrait of...",
      "imageUrl": "https://...",
      "categories": [
        {"slug": "photography", "title": "Photography"},
        {"slug": "portrait-selfie", "title": "Portrait Selfie"}
      ],
      "source": {"authorName": "栗"},
      "studioUrl": "https://image2studio.com/studio?prompt=..."
    }
  ]
}
```

## 参数

| 参数 | 短参 | 说明 |
|---|---|---|
| `keywords` | | 搜索关键词（位置参数，也从 stdin 读取） |
| `--limit` | `-l` | 简单模式：返回 N 条结果，默认 8（与 `--page` 互斥，1-based） |
| `--page` | `-p` | 翻页模式：第几页，1-based（与 `--limit` 互斥） |
| `--page-size` | | 翻页时每页数量，默认 8，最大 20 |
| `--category` | | 按分类过滤（slug 值，如 `photography`、`portrait-selfie`） |
| `--featured` | | 只看精选提示词 |
| `--json` | | 输出 JSON 格式（默认 Markdown） |
| `--save` | | 下载参考图片到本地目录 |
| `--output` | | 输出目录（仅 `--save` 时生效，图片存到 `{output}/ideas/images/`） |

## 常用搜索词示例

| 搜索词 | 场景 |
|---|---|
| `cinematic portrait` | 电影感人像 |
| `product photography` | 产品摄影 |
| `luxury perfume` | 奢侈品/香水广告 |
| `cyberpunk city` | 赛博朋克城市 |
| `minimal poster` | 极简海报 |
| `anime character` | 动漫角色 |
| `food photography` | 美食摄影 |
| `fashion editorial` | 时尚大片 |
| `电商海报` | 中文电商场景 |
| `水墨风格` | 中国风 |

## 图片保存

`--save` 参数将参考图片下载到本地，图片保存在 `{output_dir}/ideas/images/` 目录下，Markdown 输出中的图片引用自动切换为本地路径：

```bash
# 搜索并下载参考图
apimart-cli ideas "product photography" --save

# 指定输出目录
apimart-cli ideas "cat" --save --output ./my-ideas
```

```bash
# 管道组合：搜索 → 存文件
apimart-cli ideas "portrait" --json --save > results.json

# 搜索 → 用 jq 提取 prompt → 生成图片
apimart-cli ideas "cat" --json \
  | jq -r '.results[0].prompt' \
  | apimart-cli image --model gpt-image-2 --prompt -
```

## 注意事项

- 无需配置 API Key 即可使用
- 数据来源于 Image2Studio 公开 API
- 参考图版权归原作者所有，仅作灵感参考
