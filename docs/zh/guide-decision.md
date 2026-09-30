# 决策模型指南

`aigc-cli decision`（别名 `decide`、`systemone`）对一段文本运行"决策类"问题（单选 / 是非 / 评分），并返回带概率的结构化答案。本命令**不**生成自然语言文本。响应为 JSON，可直接管道给 `jq` 得到可读视图（见[输出](#输出)）。

## 命令

```
aigc-cli decision [flags]
```

## 参数

| 参数 | 说明 |
|---|---|
| `--json <path\|inline-json\|->` | 题库（question bank）：JSON 或 JSONC 文件。支持文件路径、内联 JSON 字符串、`-` 读 stdin。缺省回退到 `defaults.decision.bank` |
| `--questions a,b,c` | 仅回答指定的问题（ASCII 逗号分隔，可重复；题目名可用中文）。省略则回答题库中**所有**问题 |
| `--state <text\|path\|->` | 待判定的素材。支持直接文本、文件路径、`-` 读 stdin。若内容是合法 JSON 对象/数组，按结构化 state 发送；否则按纯字符串发送 |
| `--list` | 列出题库中所有问题的名称后退出 |
| `--dry-run` | 打印等价的 curl 请求，不真正调用 API |
| `-P, --provider` | （全局）切换命名 provider，例如 `ollama` 或 `openrouter` |
| `-m, --model` | （全局）切换模型，例如 `tev1`、`tev1:0.8b`、`nimble`、`typesafe/jev-latest` |

## 题库 JSON

题库文件是 JSON 或 JSONC（允许 `//` 注释和尾逗号）。顶层字段：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `model` | string | 否 | 覆盖默认模型 |
| `state` | string / object / array | 否 | 默认 state，可被 `--state` 覆盖 |
| `questions` | object | 是 | 命名问题字典，1–64 个问题 |

每个问题字段：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | `choice` / `noul` / `score` | 是 | 问题类型 |
| `instructions` | string | 是 | 问题正文 |
| `criteria` | object / array | 视类型而定 | `choice`：`{ 选项: 描述 }`；`score`：`[低→高, …]`（从低到高）；`noul`：可省略 |

完整示例见 [examples/decision/triage.json](examples/decision/triage.json)，配套素材见 [examples/decision/state.txt](examples/decision/state.txt)。

### 三类问题与返回

| 类型 | 返回字段 |
|---|---|
| `choice` | `choice`（选中的选项 key）、`probabilities`（每选项概率）、`confidence` |
| `noul` | `noul`（答案为"是"的概率，0–1）。**没有** `confidence` 字段 |
| `score` | `score`（按概率求和得到的分档，可能为小数）、`legend`（索引→标签）、`probabilities`、`confidence` |

> `confidence` 衡量概率分布的集中程度，**不是**答案正确的概率。

## 输出

响应为 JSON（`model` / `answers` / `usage`），可直接管道给 `jq`：

```bash
# 取某题选中的选项
# 注意：题库用中文键时，jq 必须用方括号取值；.answers.意图.choice 会报语法错误
aigc-cli decision --json triage.json --state state.txt | jq -r '.["answers"]["意图"].choice'

# 每题一行（键无关，中英文题库通用；三个字段互斥，用 // 依次回退）
aigc-cli decision --json triage.json --state state.txt \
  | jq -r '.answers | to_entries[] | "\(.key): \(.value.choice // .value.noul // .value.score)"'

# 只在模型确信时采用，否则升级人工
aigc-cli decision --json triage.json --state state.txt \
  | jq -r 'if .answers["意图"].confidence > 0.85 then .answers["意图"].choice else "ESCALATE" end'
```

## 支持的 Provider

不同 provider 的端点路径**不同**。本项目**仅端到端验证** Ollama 与 OpenRouter；其余按官方文档记录但**本项目未测试**。

| Provider | 官方文档 | Configured `base_url` | 完整端点 |
|---|---|---|---|
| TypeSafe AI | https://docs.typesafe.ai/api | https://api.typesafe.ai | POST https://api.typesafe.ai/v1/systemone |
| OpenRouter | https://openrouter.ai/docs/guides/community/typesafe-sdk | https://openrouter.ai/api/v1 | POST https://openrouter.ai/api/v1/systemone |
| Ollama (>= 0.35.0) | https://ollama.com/library/tev1 | http://localhost:11434 | POST http://localhost:11434/v1/systemone |
| LLM Gateway | https://docs.llmgateway.io/features/system-one | https://api.llmgateway.io/v1 | POST https://api.llmgateway.io/v1/systemone |
| LiteLLM 代理 | https://docs.litellm.ai/docs/pass_through/typesafe | `{proxy}/typesafe` | POST `{proxy}/typesafe/v1/systemone` |

- **Ollama** 无需 API Key，本地运行。
- **TypeSafe AI** 与 **OpenRouter**（以及其他）需要 API Key。

## 优先级

```
CLI 参数 > JSON（题库） > defaults.decision YAML > 代码默认值
```

`--json` 与其他 flag 同时出现时：每个**显式设置**的 flag 覆盖题库对应字段；未触及的字段保持题库原值。仅传 `--json` 不带任何 flag 时，题库**整体发送**。

## 示例

> 以下示例假设已配置 `defaults.decision.provider`（如 `ollama`）。若未配置，请加 `-P ollama`（本地）或 `-P openrouter`。

```bash
# 本地 Ollama（免费、离线）
aigc-cli decision --json triage.json --questions 意图,退款 --state state.txt

# 切换到 OpenRouter 上的远程 Jev 模型
aigc-cli decision -P openrouter -m typesafe/jev-latest --questions 意图 --state state.txt

# 更小的本地模型（用 config 里的默认题库）
aigc-cli decision -m tev1:0.8b --state state.txt

# 结构化 state（对象）—— 标注上下文段落
aigc-cli decision --questions 退款 \
  --state '{"policy":"退款需在 30 天内提出。","request":"我 12 天前买的，想退款。"}'

# stdin、列清单、dry-run
cat ticket.txt | aigc-cli decision --questions 意图
aigc-cli decision --list
aigc-cli decision --questions 意图 --state state.txt --dry-run
```

## 注意事项

- `choice` / `score` 接受 2–26 个选项；tev1 训练时覆盖 2–24，**请保持在 2–24**
- 单次请求 1–64 个问题，请求体 ≤ 64 KiB
- tev1 上下文约 **2000 tokens**，请保持 state 简短
- 若没有合适选项，**务必**添加 `none` / `other` 选项——模型不会自动说"以上皆非"
- tev1 基模为 **Qwen3.5**（`ollama show tev1` → `arch qwen35`），中文题库 / 中文 state 实测可用（见 [examples/decision/triage.json](examples/decision/triage.json)）；但厂商评测以英文为主，非英文**无质量保证**——生产上请配合 `confidence` 阈值与人工兜底
- prompt injection 在厂商测试中覆盖有限：state 要当**数据**看，不要让它左右下游动作
- 决策模型可能出错——**不要**把它当作高风险决策的唯一依据
- 本命令**尚未**接入 MCP

## Config

```yaml
defaults:
  decision:
    provider: ollama            # config.providers 里定义的命名 provider（ollama / openrouter / ...）
    model: tev1                 # tev1 | tev1:0.8b | nimble | typesafe/jev-latest
    bank: ~/exams/triage.json   # 默认题库（--json 缺省时使用）
```