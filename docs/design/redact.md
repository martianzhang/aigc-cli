# `redact` 设计稿（v0.1）

> 状态：**v0.1 已实现**（规则引擎、scan/apply/restore/check/rules、mask、mapping、chatner、tokencls、dictionary、MCP）。  
> 目标读者：维护者 / 贡献者。  
> 用户文档：`docs/zh/guide-redact.md` 与 `docs/en/guide-redact.md`。

### 实现状态与设计稿的差异

- **已实现**：纯规则引擎 + 校验位、`scan/apply/restore/check/rules/init`、占位符/掩码/涂黑/hash、映射读写（含加密）、别名归并、`dictionary`、`chatner`（Ollama / 任意 OpenAI 兼容端点）、`tokencls`（本地 ONNX）。
- **M3 已实现**：`tokencls` ONNX 适配器（滑窗 + BIO/BIOES）、`dictionary` 精确词表、MCP `redact_text` / `redact_check`。ONNX 模型不随二进制分发，用户把 `model.onnx` + `config.json` + `tokenizer.json` 放进 `models/redact/<model>/`。
- **输出**：`-o/--output` 保持“输出目录”语义；`apply` 写 `<output>/<stem>.redacted<ext>`，`restore` 写 `<output>/<stem>.restored<ext>`，`--stdout` 用于管道。
- **mask 差异**：内置类型优先使用自带策略，`defaults.redact.mask.overrides` 覆盖；全局 `defaults.redact.mask` 只作为自定义/未知类型的默认，避免全局 `keep_prefix: 3` 悄悄削弱身份证的 6+4 默认。

---

## 0. 目标与非目标

### 目标

1. **默认离线、零模型可用**：只靠内置正则 + 校验位，就能覆盖身份证、手机、银行卡、邮箱、统一社会信用代码、案号、车牌、护照、律师执业证号等确定性 PII。
2. **可复核**：识别与替换分离；`scan` 输出 span 列表，人可以改、可以补、可以取消。
3. **可逆**：默认占位符 + 映射字典，支持 `restore`；映射文件可加密。
4. **可组合**：`scan → apply → restore → check` 四个命令可以单独用，也可以串进管道。
5. **可选模型**：本地 ONNX 或本机 OpenAI 兼容端点（推荐 Ollama；HaS / llama-server 均可）作为第二层召回，不联网、不作为默认依赖。
6. **纯文本**：`txt / md / csv / json / stdin`。v1 不处理 Word、PDF、图片。

### 非目标

- 不做 Word/PDF/图片/OCR 的格式保真脱敏；
- 不做“合规认证”承诺，文档必须写明“辅助工具，需人工复核”；
- 不内置云端 NER 服务；
- 不把映射字典上传或同步到任何地方；
- 不默认开启日期/金额等语义实体的替换（会改变文本语义，需显式开启）。

---

## 1. 调研结论（决定设计的前提）

第一轮对比（13 篇合成法律文书、4,428 字、223 个实体）：

| 方案 | 体积 | 速度 | 核心 P | 核心 R | 核心 F1 |
|---|---|---:|---:|---:|---:|
| 纯正则 L0 | 0 | 4.4 ms / 4.4k 字 | 1.000 | 0.419 | 0.591 |
| privyscope-zh-hans（ONNX） | 98 MB | 6,600 字/秒 | 1.000 | 0.581 | 0.735 |
| Horizon pii-redactor-small（ONNX q8） | 268 MB | 4,000 字/秒 | 0.992 | 0.704 | 0.824 |
| HaS 4.0 0.6B（Q4, llama.cpp / Ollama） | 378 MB | 199 字/秒 | 0.989 | 0.995 | 0.992 |
| 正则 + HaS | 378 MB | 由 HaS 决定 | 0.989 | 1.000 | 0.995 |

由此确定四条设计原则：

1. **正则必须是第一层，而不是兜底。** 确定性字段上它就是 100%，且零成本。
2. **模型输出必须过校验位。** Horizon 会把一个身份证号在中间从 `NATIONAL_ID` 切成 `TAX_ID`；HaS 会把全角手机号漏掉。模型只负责“提名”，`mod 11-2 / mod 31 / Luhn` 负责“定性”。
3. **HaS 适合做 `check`，不适合做默认替换器。** 它返回实体值而不是 offset，全文回搜会把“高峰时段”里的“高峰”也替换掉；慢（199 字/秒），需要本机模型服务（Ollama / llama-server）。
4. **不要同时上两个神经网络。** “正则 + HaS”已经 0.995；再叠加 Horizon/privyscope 没有增益，反而多误报。

---

## 2. 命令面

```bash
# 只识别，不修改文件
aigc-cli redact scan 起诉状.txt
aigc-cli redact scan 起诉状.txt --json > spans.json
cat 起诉状.txt | aigc-cli redact scan - --json

# 默认：识别 + 替换一步到位，不需要先 scan
aigc-cli redact apply 起诉状.txt --mapping 起诉状.map.json --style placeholder

# 可选：先 scan，人工改完 spans.json 再 apply
aigc-cli redact scan 起诉状.txt --json > spans.json
aigc-cli redact apply 起诉状.txt --stdout --spans spans.json --mapping 起诉状.map.json

# 还原（仅 placeholder 风格可靠）
aigc-cli redact restore 起诉状.redacted.txt --mapping 起诉状.map.json

# 残留检查：用映射里的原文回扫输出，命中即报警
aigc-cli redact check 起诉状.redacted.txt --mapping 起诉状.map.json

# 查看/导出当前生效的规则（内置 + 覆盖 + 自定义）
aigc-cli redact rules
aigc-cli redact rules --json
aigc-cli redact rules --export > rules-audit.yaml   # 信息性导出，不是可直接回灌的 rules-file

# 查看规则数量和可选模型配置说明（v0.1 不下载模型）
aigc-cli redact init
```

### 2.1 退出码

| 码 | 含义 |
|---|---|
| 0 | 成功；`check` 表示未发现残留 |
| 1 | 运行时错误（文件、配置、模型、网络） |
| 2 | `check` 发现残留（便于 CI `if aigc-cli redact check ...; then`） |

### 2.2 输入输出约定

- 位置参数 `-` 表示 stdin；`--stdout` 表示 stdout。`-o/--output` 仍然是输出目录。
- `apply` 写 `<输出目录>/<stem>.redacted<ext>`，`restore` 写 `<输出目录>/<stem>.restored<ext>`；绝不覆盖输入文件。
- `scan` 默认输出人类可读表格到 stdout；`--json` 输出机器可读 JSON。
- **`apply` 不要求先 `scan`**：不带 `--spans` 时，等价于“按当前配置识别并替换”。`--spans` 只是可选的复核覆盖。

| 调用 | 行为 |
|---|---|
| `redact apply f.txt` | 直接识别 + 替换，写 `<输出目录>/f.redacted.txt`（默认，最常用） |
| `redact apply f.txt --spans spans.json` | 只替换 `spans.json` 里的 span（人工复核后） |
| `redact apply f.txt --stdout` | 输出到 stdout |

- `apply --with-date` / `--with-amount` 可以在单次运行中开启日期/金额替换（默认关闭）。
- 所有日志/进度走 stderr，保证 `--stdout` 可以干净地接管道。

---

## 3. 引擎架构

```
┌─────────────────────────────────────────────────────┐
│ L0 正则       内置规则 + 校验位 + 边界 + 全角折叠      │
│ L1 词表       黑名单（必脱）/ 白名单（必不脱）/ 自定义  │
│ L1.5 归一     同一实体同一代号（精确值 + 别名表）       │
│ L2 模型       可选：ONNX 分类器 / HaS（OpenAI 兼容）    │
│ L3 收口       重叠消解 → 占位符编号 → 映射 → 输出       │
└─────────────────────────────────────────────────────┘
```

### 3.1 引擎接口

```go
// internal/redact/engine.go
type Engine interface {
    Name() string
    Detect(ctx context.Context, doc *Document) ([]Span, error)
}
```

- `RuleEngine`：L0 + L1，纯函数、无 I/O。
- `ModelEngine`：包装一个 `ModelAdapter`（ONNX / OpenAI 兼容），负责分块、偏移映射、标签归一。
- `Pipeline`：按配置组合多个 Engine，做校验、去重、重叠消解。

### 3.2 组合策略

```go
type Pipeline struct {
    Rules   *RuleEngine
    Models  []Engine   // 0..n，按配置
    Allow   []string
    Deny    []string
}
```

合并顺序固定：

1. `Rules.Detect`；
2. 每个 `Model.Detect`；
3. **模型 span 必须通过对应校验器**（若类型有校验器），否则丢弃；
4. `deny` 追加（命中必脱）；`allow` 删除（命中必不脱）；两者冲突在加载配置时报错；
5. 重叠消解：
   - 规则 span 优先于模型 span（结构化类型）；
   - 同类型长 span 优先；
   - 仍冲突时按 `priority` 降序、规则定义顺序升序；
6. 同一实体值分配同一个占位符。

### 3.3 引擎选择

```yaml
defaults:
  redact:
    engine: rules        # rules | model | auto
```

| 值 | 行为 |
|---|---|
| `rules` | 只跑 L0+L1；零模型、默认 |
| `model` | 只跑模型（不推荐；确定性字段会退化） |
| `auto` | 有可用 provider 时 rules + model；否则等价 `rules` |

模型 provider 复用现有优先级链：

```
--provider/--api-base/--api-key > defaults.redact.provider > 全局 api_key/base_url
```

- **ONNX**：`providers.<name>.type: local` + `models_dir`；模型放 `models_dir/redact/<model>/`（`model.onnx` + `config.json` + `tokenizer.json`），细节见 `defaults.redact.tokencls`。
- **本机 chat 模型（推荐 Ollama）**：`providers.ollama.type: ollama` + `base_url: http://127.0.0.1:11434/v1`。HaS 4.0 的 GGUF 用 `ollama create` 导入；同一套适配器也能接 llama-server / vLLM / LM Studio 等任意 OpenAI 兼容端点。
- **安全限制**：`engine: model` 且 endpoint 非 loopback 时，必须显式 `--allow-remote`，否则报错。防止把原文误发到公网。

把模型做成 Provider 的好处：

1. 不引入 `llama-server` 这种专用 sidecar；Ollama 的生命周期、模型拉取、GPU/CPU 调度都由用户已有的工具负责。
2. `--provider` / `defaults.redact.provider` / `config.yaml` 的优先级链直接复用，和其他命令一致。
3. 以后接更强的模型（Qwen、GLM、DeepSeek…）只改 `provider`/`model`/`prompt`，不改引擎代码。

HaS 4.0 通过 Ollama 导入：

```bash
cat > Modelfile <<'EOF'
FROM ./has_4.0_0.6B_q4.gguf
EOF
ollama create has4 -f Modelfile
ollama serve            # 默认 127.0.0.1:11434
```

```yaml
providers:
  ollama:
    type: ollama
    base_url: "http://127.0.0.1:11434/v1"

defaults:
  redact:
    provider: ollama
    model: has4
```

### 3.4 模型适配器

```go
type ModelAdapter interface {
    Detect(ctx context.Context, doc *Document) ([]Span, error)
    LabelMap() map[string]string // 原始标签 -> 规范类型
}
```

内置适配器：

| 适配器 | 输入 | 输出 | 说明 |
|---|---|---|---|
| `tokencls` | ONNX + tokenizer.json | 带 offset 的 span | privyscope / Horizon 类；`redact init --model` 从 aigc-cli-models 下载（SHA256 + LICENSE） |
| `chatner` | 任意 OpenAI 兼容 chat | 实体值 | 回搜 + 上下文锚定；HaS 4.0 是推荐 preset，不是特例 |
| `dictionary` | 本地词表 | 带 offset 的 span | 无模型时的 ORG/项目名兜底 |

`chatner` 是通用的“让模型输出实体清单”适配器：

```yaml
defaults:
  redact:
    model_engine: auto           # auto | chatner | tokencls
    chatner:
      occurrence: anchored       # anchored | all | first
      anchor_window: 20
      types: ["人名", "组织", "地址", "电话", "身份证号", "银行卡号",
              "邮箱", "统一社会信用代码", "案号", "车牌号", "护照号",
              "日期", "金额", "律师执业证号"]
      prompt: ""                 # 空 = 内置 NER 模板；可换成任意模型的 prompt
```

- 换模型只改 `defaults.redact.provider` / `model`，或 `--provider`；
- 不同模型只要按约定返回 `{"类型": ["实体", ...]}` 即可，解析层做容错（代码块、前后缀、JSON 修复）；
- 更复杂的模型可以只用于 `check`，不影响默认的正则链路。

**值级模型的消歧（以 HaS 为例，解决 `高峰时段`）**：

- `chatner.occurrence: anchored | all | first`，默认 `anchored`；
- `anchored`：只在实体值前后 `±20` 字内出现角色词（原告/被告/申请人/联系人/姓名/法定代表人/委托代理人/住址/电话…）时替换；
- 找不到锚点的出现位置进入 `needs_review`，**不进入 `spans`**，因此 `apply` 不会改动它；
- 需要人工确认时，先 `scan --json`、改 `spans.json`，再 `apply --spans`。

---

## 4. 配置设计

配置统一放在 `defaults.redact`；provider/model 与规则、引擎配置同处一段。

```yaml
defaults:
  redact:
    provider: ollama             # 引用 providers.ollama；不配则 engine 自动退回 rules
    model: has4                  # Ollama 里的模型名（HaS 4.0 或任意更强模型）
    # ── 全局默认 ──
    style: placeholder            # placeholder | mask | black | hash
    placeholder: "<{type}_{n}>"   # 占位符模板
    engine: rules                 # rules | model | auto
    date: false                   # 是否把 DATE 当敏感实体（默认关）
    amount: false                 # 是否把 AMOUNT 当敏感实体（默认关）

    # ── 内置规则开关（只写要改的；未出现的用内置默认）──
    rules:
      id_card: true
      phone_mobile: true
      phone_landline: true
      bank_card: true
      email: true
      uscc: true
      case_no: true
      lawyer_no: false
      plate: false
      passport: false
      ip: false
      url: false
      postcode: false

    # ── 覆盖内置规则（只改想改的字段）──
    overrides:
      phone_mobile:
        style: mask
      bank_card:
        regex: '[0-9]{12,19}'
        priority: 50

    # ── 自定义规则（列表，保序）──
    custom:
      - name: contract_no_custom
        type: CONTRACT_NO
        regex: '(?i)HT[-_]?[0-9]{4}[-_][0-9]{6}'
        style: mask
        validator: none
        boundary: word
        group: 0
        priority: 100
        max_len: 64
        enabled: true
        mask:
          strategy: fixed
          position: middle
          keep_prefix: 2
          keep_suffix: 3

    # ── 精确词表 ──
    allow: ["北京市", "中国工商银行"]   # 命中也不脱
    deny:  ["某某项目"]                 # 命中必脱

    # ── 别名（同一实体不同写法，共用代号）──
    aliases:
      "北京云创智能科技有限公司": ["云创智能", "CloudGenius"]
      "张伟": ["老张", "张某某"]

    # ── mask 策略（内置类型默认 + 按类型覆盖；全局仅用于自定义/未知类型）──
    mask:
      strategy: fixed
      position: middle
      keep_prefix: 3
      keep_suffix: 4
      keep_prefix_regex: ""
      keep_suffix_regex: ""
      char: "*"
      mask_len: 3
      preserve_length: true
      keep_separators: true
      overrides:
        ID_CARD:    { keep_prefix: 6, keep_suffix: 4 }
        PHONE:      { keep_prefix: 3, keep_suffix: 4 }
        BANK_CARD:  { position: prefix, keep_suffix: 4 }
        EMAIL:      { keep_prefix: 1, keep_suffix_regex: "@.*$" }
        PERSON:     { position: suffix, keep_prefix: 1, strategy: name }
        ADDRESS:    { position: suffix, keep_prefix_regex: "^.*?(?:省|市|区|县|镇|街道)" }
        ORG:        { position: all }

    # ── 精确词表（无模型时的 ORG/项目名兜底）──
    dictionary:
      - value: "北京云创智能科技有限公司"
        type: ORG
    # 大列表用文件；相对 config.yaml 所在目录；推荐 TSV
    dictionary_files:
      - "./dict/orgs.tsv"
      - "./dict/projects.json"

    # ── 模型引擎（可选）──
    model_engine: auto             # auto | chatner | tokencls
    chatner:
      occurrence: anchored         # anchored | all | first
      anchor_window: 20
      types: ["人名", "组织", "地址", "电话", "身份证号", "银行卡号",
              "邮箱", "统一社会信用代码", "案号", "车牌号", "护照号",
              "日期", "金额", "律师执业证号"]
      prompt: ""                   # 空 = 内置 NER 模板
    tokencls:
      model: zh-pii                # models/redact/zh-pii/
      max_len: 512
      stride: 128
      operating_point: high_recall   # high_recall | balanced | high_precision
      # inputs: ["input_ids", "attention_mask", "token_type_ids"]
      # output: "logits"


```

### 4.1 字段语义

| 字段 | 类型 | 说明 |
|---|---|---|
| `style` | string | `placeholder` / `mask` / `black` / `hash` |
| `placeholder` | string | 必须包含 `{type}` 与 `{n}`；默认 `<{type}_{n}>` |
| `engine` | string | `rules` / `model` / `auto` |
| `model_engine` | string | `auto` / `chatner` / `tokencls`（`--model-engine` 同义） |
| `config_file` | string | 外部 redact YAML；相对 config.yaml 目录；作为覆盖层合并 |
| `profile` / `profiles` | string / map | 命名外部配置；`--profile` 单次切换 |
| `rules` | map[string]bool | 内置规则启停；未知 name 报错 |
| `overrides` | map[string]patch | 覆盖内置规则的 regex/style/validator/boundary/group/priority/max_len |
| `custom` | []Rule | 自定义规则；name 与内置重名报错 |
| `allow` / `deny` | []string | 精确匹配；交集报错；`deny` 优先于 `allow` |
| `aliases` | map[string][]string | 别名归并；同一组共用占位符 |
| `dictionary` | []{value,type} | 小型精确词表；无模型时的 ORG/项目名兜底 |
| `dictionary_files` | []string | TSV/CSV/JSON 词表文件；相对 config.yaml 目录；推荐 TSV |
| `tokencls` | object | 本地 ONNX 模型目录、max_len/stride、inputs/output 覆盖 |
| `mask` | MaskPolicy | 仅 `style: mask` 时生效 |
| `chatner` | object | 值级模型的 `types` / `prompt` / `occurrence` / `anchor_window` |
| `date` / `amount` | bool | 是否让模型/自定义规则输出的 DATE/AMOUNT **参与替换**；内置**不提供** DATE/AMOUNT 正则（语义类，误伤大）。`chatner.types` 里可以保留它们用于报告，但默认只报告不替换 |

### 4.2 优先级

```
CLI flags > defaults.redact.* 配置 > 内置默认
```

- `--only a,b`：只启用列出的规则（覆盖 `rules` 开关）；
- `--disable a,b`：临时关闭；
- `--style` / `--engine` / `--provider`：单次覆盖；
- `--rules-file`：用外部 YAML 替换 `rules`/`overrides`/`custom` 三节（便于团队共享）。

### 4.3 加载时校验（失败即报错，不静默）

- 未知的规则 name / override / alias key；
- 正则编译失败（错误里带上规则名）；
- `group` 越界；
- `validator` / `boundary` / `strategy` / `position` 非法值；
- `allow` 与 `deny` 有交集；
- `placeholder` 缺 `{type}` 或 `{n}`；
- `custom.name` 与内置重名（提示改用 `overrides`）。

### 4.4 最小配置示例

只想关掉银行卡规则：

```yaml
defaults:
  redact:
    rules:
      bank_card: false
```

也可以用命令改标量：

```bash
aigc-cli config set defaults.redact.rules.bank_card false
```

`custom` 是列表，`config set` 不支持，需编辑文件或用 `--rules-file`。

### 4.5 外部配置与 profile

完整配置可以放到独立 YAML，作为 `defaults.redact` 的覆盖层：

```yaml
defaults:
  redact:
    profile: strict
    profiles:
      strict: ./redact/strict.yaml
      loose: ./redact/loose.yaml
    # 或单文件：config_file: ./redact/default.yaml
```

- 配置内相对路径按 config.yaml 目录解析，支持 `~/`；CLI `--redact-config` 按当前目录；
- 外部文件可以是裸 redact 段，或包在 `redact:` / `defaults.redact:` 下；
- 合并语义：map 按键递归合并，标量/列表替换；
- 优先级：内置默认 < `defaults.redact` < 外部文件/profile < `--rules-file` < 其他 CLI 参数；
- `--profile <name>` / `--redact-config <path>` 只影响本次命令，不改主配置。

---

## 5. 内置规则 v1

### 5.1 规则目录

内置规则按元典的四个类别组织；`redact rules` 输出 `category` 列。

| 类别 | 默认启用 | 规则 |
|---|---|---|
| 个人信息 | ✅ | `id_card` / `id_card_15` / `phone_mobile` / `phone_landline` / `phone_400` / `email` / `passport_ctx` / `travel_permit` / `vin` / `wechat` / `wxid` / `qq` / `plate` / `lawyer_no_ctx` / `medical_no` / `employee_no` / `engine_no` / `postcode` / `cert_no` |
| 个人信息 | ❌ | `passport`（裸 E/G/D+8 位）、`lawyer_no`（裸 17 位） |
| 单位与机构 | ✅ | `uscc` / `business_reg_no` |
| 金融与网络 | ✅ | `bank_card` / `bank_account` / `jp_corporate_account` / `mac` / `stock_code` / `invoice_code` |
| 金融与网络 | ❌ | `ip` / `url` / `amount` |
| 文书与日期 | ✅ | `case_no` / `contract_no` / `doc_no` / `property_cert_no` / `construction_permit_no` / `policy_no` / `copyright_no` |
| 文书与日期 | ❌ | `date_cn` |

说明：

- 带“标签”的规则（`passport_ctx`、`lawyer_no_ctx`、`bank_account`、`contract_no` 等）使用 `Group: 1`，只替换标签后的值，不替换“护照号：”这类前缀。
- 裸 `passport` / `lawyer_no` / `ip` / `url` / `date_cn` / `amount` 默认关闭，避免误伤；按需在 `defaults.redact.rules` 打开。
- 语义类（姓名、地址、机构、性别、国籍、民族、年龄、宗教信仰、生物识别、健康、境外法院）仍由 `dictionary` / 模型负责，不放进正则。

### 5.2 三条硬规则（来自第一轮踩坑）

1. **边界只认 ASCII 字母数字。**  
   Go RE2 没有 lookbehind；用命中区间左右字符判断。**不能用 `unicode.IsLetter` / `unicode.IsDigit`**：中文紧邻数字时会被误判为词边界，导致 `电话13812345678。` 里的手机号被丢弃。

2. **匹配前做 1:1 全角折叠。**  
   全角 ASCII（`０-９`、`Ａ-Ｚ`、`．`…）映射到半角，**字符数不变**，所以 offset 不变。校验器也要先折叠再算，否则全角手机号/身份证会被判非法。

3. **校验器接收“原始匹配串”，内部自行去分隔符。**  
   手机 `+86 139-1234-5678`、银行卡 `6222 0202 ...`、身份证 `110105 19850312 1234` 都要先 strip 再校验；**不要改 span 的原文**。

### 5.3 正则草稿

```go
// id_card（18 位，校验位另算）
`[1-9][0-9]{5}(?:19|20)[0-9]{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12][0-9]|3[01])[0-9]{3}[0-9Xx]`
// id_card_15
`[1-9][0-9]{5}[0-9]{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12][0-9]|3[01])[0-9]{3}`
// phone_mobile
`(?:\+?86[-\s]?)?1[3-9][0-9](?:[-\s]?[0-9]{4}){2}`
// phone_landline
`0[0-9]{2,3}[-\s]?[2-9][0-9]{6,7}(?:-[0-9]{1,4})?`
// phone_400
`(?:400|800)[-\s]?[0-9]{3}[-\s]?[0-9]{4}`
// bank_card（13–19 位，允许分隔）
`[0-9](?:[ -]?[0-9]){12,18}`
// email
`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`
// uscc
`[0-9A-HJ-NPQRTUWXY]{2}[0-9]{6}[0-9A-HJ-NPQRTUWXY]{10}`
// case_no（近似；覆盖 民初/民终/刑初/执/行初）
`[（(][0-9]{4}[）)][\p{Han}]{1,4}[0-9]{2,6}[\p{Han}]{1,8}[0-9]{1,8}号`
// plate
`[京津沪渝冀豫云辽黑湘皖鲁新苏浙赣鄂桂甘晋蒙陕吉闽贵粤青藏川宁琼][A-Z][A-Z0-9]{5,6}`
// passport
`[EGD][0-9]{8}`
```

### 5.4 重叠消解

示例：

- 18 位身份证 vs 16 位银行卡：ID `priority=100`、bank `90`，ID 胜；
- 手机号 vs 银行卡：手机 `70`、bank `90`，若 11 位片段通过 Luhn 才可能冲突；优先长匹配；
- 案号里包含日期/数字：案号 `85`、日期规则默认关闭；开启时按长匹配优先。

算法：

```
deny 优先；其余按 (length desc, priority desc, start asc, rule order asc)
greedy accept if no overlap with accepted
```

长 span 优先意味着：一个包含手机号的机构名会整体替换，而不是只替换里面的手机号——覆盖更彻底，也避免“半截机构名”留在文本里。

---

## 6. 数据模型

### 6.1 Span / Entity / Document

```go
// internal/redact/types.go
type Span struct {
    Start  int    // byte offset（内部）
    End    int    // byte offset（内部，exclusive）
    Type   string // ID_CARD, PERSON, ...
    Text   string // 原文
    Rule   string // 规则名或模型名
    Source Source // rule | model | deny | manual
    Score  float64
}

type Entity struct {
    Type        string
    Value       string
    Placeholder string
    Count       int
}

type Document struct {
    ID      string // 文件名或 stdin
    Text    string
    RuneMap []int  // byte offset -> rune offset（对外输出用）
}
```

### 6.2 占位符

- 默认 `<{type}_{n}>`，例如 `<PERSON_1>`、`<ID_CARD_1>`、`<ORG_2>`。
- `n` 按**首次出现顺序**分配；同一实体值（含 `aliases` 归并）共用同一个 `n`。
- 跨文件：v0.1 一个映射文件对应一个输出文档；跨文件沿用编号（脱敏空间）在后续版本实现。
- 模板只支持 `{type}` 与 `{n}`；默认英文类型，便于下游模型理解。

### 6.3 映射文件

```json
{
  "version": 1,
  "created_at": "2026-11-10T12:00:00Z",
  "doc": "起诉状.txt",
  "style": "placeholder",
  "placeholder": "<{type}_{n}>",
  "source_sha256": "…",
  "output_sha256": "…",
  "entities": [
    {"type": "PERSON", "value": "张伟", "placeholder": "<PERSON_1>",
     "aliases": ["老张"], "count": 3}
  ],
  "occurrences": [
    {"doc": "起诉状.txt", "start": 10, "end": 12,
     "placeholder": "<PERSON_1>", "masked": ""}
  ]
}
```

- `entities` 用于 `check`（拿 value + aliases 回扫）和审计；
- `occurrences` 用于 `restore`（按位置顺序替换；`start/end` 是**输出文档**的 rune offset）；
- `output_sha256` 用于确认“输出没被改过”；不一致时 `restore` 拒绝（除非 `--force`）；
- `source_sha256` 用于审计原始文档；
- 对外 JSON 的 `start/end` 一律是 rune offset，内部是 byte offset；
- 可选加密：`--encrypt` 时整个映射用主密钥加密，文件写成
  `{"version":1,"encrypted":true,"payload":"enc:v1:…"}`；
- v0.1 一个映射文件对应一个输出文档；跨文件共享编号（脱敏空间）在后续版本实现。

### 6.4 别名与共指

v1 只做**精确值 + 用户别名表**：

```yaml
aliases:
  "北京云创智能科技有限公司": ["云创智能", "CloudGenius"]
```

- 同一组内的值共用占位符；
- 自动共指（张三/张某某/老张）不在 v1 做，交给模型或用户表；
- 模型返回的 alias 分组（HaS 的 `[ID]`）在 v2 考虑接入。

---

## 7. 文本处理细节

### 7.1 rune vs byte

- 内部一律 byte offset（Go 原生，切片快）；
- 对外 JSON 一律 rune offset（跨语言友好，和 Python/JS 的字符串索引一致）；
- 提供 `text.RuneIndex` 预计算，O(1) 转换；
- CRLF：读入后统一成 `\n` 处理，输出时按 `--keep-crlf` 恢复（默认保持原样）。

### 7.2 分隔符与全角

- 匹配前：1:1 全角折叠（不改变字符数）；
- 校验前：去空格、`-`、`·`、`/`、`\`、`.`（按类型）；
- 替换时：使用原始 span 文本，不用折叠后的文本。

### 7.3 大文档

- 正则：整篇跑，RE2 线性时间；
- ONNX：512 token 窗口，stride 128，偏移映射回原文；
- HaS：3000 token/块，串行（保持 mapping 一致），块间 mapping 合并；
- 单文件上限默认 20 MB，超过报错并提示 `--max-bytes`。

### 7.4 幂等

- `apply` 的输出再次 `scan` 不应再命中（占位符/掩码不能匹配规则）；
- 测试必须覆盖：`apply(apply(x)) == apply(x)`。

---

## 8. 输出与 schema

### 8.1 `scan --json`

```json
{
  "version": 1,
  "doc": "起诉状.txt",
  "sha256": "…",
  "offset": "rune",
  "spans": [
    {"start": 10, "end": 12, "type": "PERSON", "text": "张伟",
     "source": "model:chatner", "score": 0.92}
  ],
  "stats": {"total": 1, "by_type": {"PERSON": 1}},
  "needs_review": [
    {"start": 40, "end": 42, "type": "PERSON", "text": "高峰",
     "reason": "ambiguous_occurrence"}
  ]
}
```

- `needs_review` 只由模型引擎产生（例如 HaS 的同形词），**不在 `spans` 里，`apply` 不会替换它**；
- `scan` 不写文件、不生成 mapping；
- `apply --spans` 会校验 `sha256`：文件被改过就拒绝（除非 `--force`），避免用旧 offset 改错位置。

### 8.2 `scan` 人类可读

```
TYPE       TEXT                     RULE              POS
PERSON     张伟                     person_dict       10..12
ID_CARD    110105********1234       id_card           37..55
...
3 spans (2 redact, 1 review)
```

- 默认对敏感值做 `mask` 预览（不打印完整原文），`--reveal` 才显示全文；
- 避免 `scan` 输出本身成为泄露源。

### 8.3 `check`

```
residual: 0
```
或：

```
residual: 2
  PERSON  "张伟"  at 120  (mapping: <PERSON_1>)
  PHONE   "13812345678"  at 340
```
退出码 2。

---

## 9. 安全

1. **默认零网络**：`engine: rules` 不发起任何请求。
2. **模型端点限制**：非 loopback 必须 `--allow-remote`；默认拒绝，防止误把原文发到公网。
3. **映射文件**：默认权限 `0600`；`--encrypt` 用主密钥加密；日志不打印 value。
4. **scan 预览**：默认打码，避免终端/日志泄露。
5. **临时文件**：`os.CreateTemp`，`0600`，用完删除；不写 `/tmp` 固定路径。
6. **模型完整性**：`redact init --model` 从 aigc-cli-models 下载模型，逐文件做 SHA256 校验，并安装 LICENSE/NOTICE；`--force` 才覆盖已有文件。
7. **不覆盖输入**：输出路径与输入相同时报错，除非显式 `--force`。

---

## 10. 包结构

```
cmd/
  redact.go                     # 薄层：NewCommand + RunE
internal/
  types/
    types_redact.go             # RedactConfig / RedactRule / MaskPolicy
  redact/
    types.go                    # Span / Entity / Document / Mapping / Residual
    text.go                     # 全角折叠、边界、rune/byte 索引、CRLF 兼容
    validator.go                # id_card / uscc / luhn / phone / ip / url
    rules.go                    # 内置规则、配置合并、检测、重叠消解
    mask.go                     # mask 策略与按类型默认
    mapping.go                  # Apply / Save / Load / Restore / Check
    engine.go                   # Engine / Pipeline / CanonicalType
    chatner.go                  # Ollama / OpenAI 兼容值级模型 + 分块
    tokencls.go                 # ONNX token-classification（滑窗 + BIO/BIOES）
    dictionary.go               # 精确词表兜底
    errors.go                   # ResidualError（退出码 2）
    # MCP: internal/mcp/tools_redact.go（redact_text / redact_check）
  cli/
    redact/
      redact.go                 # 命令组
      scan.go
      apply.go
      restore.go
      check.go
      rules.go
      init.go
  cli/options/
    config_defaults.go          # RedactConfig() 访问器
  mcp/
    tools_redact.go             # redact_text / redact_check 工具（可选）
```

### 10.1 接线点（实现时逐项确认）

| 位置 | 改动 |
|---|---|
| `internal/types/types_config.go` | `ConfigDefaults.Redact *RedactConfig`（唯一入口 `defaults.redact`） |
| `internal/cli/options/shared.go` | 新增 `ProviderNameRedact = "redact"`，并登记到 `CmdProviderMap` |
| `internal/cli/options/config_defaults.go` | 新增 `RedactConfig()` / `RedactDefaults()` 访问器 |
| `cmd/redact.go` | `rootCmd.AddCommand(redact.NewCommand(...))` |
| `internal/mcp/server.go` | 可选注册 `redact_text` / `redact_check` |
| `internal/tools/tools.go` | 若引入新依赖，按项目约定用 `//go:build tools` 锁定 |
| `docs/zh/examples/config.yaml` | 增加 `defaults.redact:` 示例（实现后） |
| `README.md` / `docs/*/guide-commands.md` | 增加命令索引（实现后） |

约束（SPEC）：

- 每个 `.go` 尽量 ≤200 行；
- 配置访问器 `options.RedactConfig()`，禁止长链 nil 判断；
- 所有 `RunE` 设 `SilenceUsage: true`；
- 错误 `%w` 包装、首字母小写；
- 跨平台：`filepath`、`os.UserHomeDir`、`os.CreateTemp`、CRLF 兼容。

---

## 11. 测试策略

| 层 | 用例 |
|---|---|
| 正则表 | 每条规则的正例/反例：合法值、错校验位、错误边界、超长、全角、带分隔符 |
| 边界 | 中文紧邻、标点紧邻、ASCII 字母紧邻、行首行尾 |
| 折叠 | 全角数字/字母/标点；offset 不变 |
| 重叠 | ID 套银行卡、手机套银行卡、案号套日期 |
| 黑白名单 | allow/deny 优先级、交集报错 |
| 占位符 | 同值同号、跨文件同号、别名归并 |
| 还原 | `scan → apply → restore == 原文`（placeholder） |
| 幂等 | `apply(apply(x)) == apply(x)` |
| mask | 每类型 × 每 position 的表驱动；长度 1/2/边界 |
| 映射 | JSON schema、加密/解密、sha256 不匹配 |
| 模型 | mock adapter：带 offset / 值级 / 非法 span / 超长 |
| 失败路径 | 坏正则、未知规则名、非 loopback、文件不存在 |
| 模糊 | 随机 UTF-8（含 emoji/CRLF）不 panic |
| 性能 | 1 MB 文本 < 100 ms（规则模式） |

`make cover` 关注 `internal/redact` 新增文件的覆盖率，目标 ≥85%。

---

## 12. 里程碑

| 阶段 | 内容 | 估时 |
|---|---|---|
| **M0** | 本设计稿 + 配置 schema + 评测报告 | 已完成 |
| **M1** | `internal/redact` 规则/校验/重叠/映射；`scan/apply/restore/check`；txt/md/stdin；单测 | 3–5 天 |
| **M2** | 配置加载/校验、mask 策略、`redact rules`、文档（zh/en）、MCP 工具 | 2–3 天 |
| **M3** | `tokencls` ONNX、`dictionary`、`chatner`（Ollama/OpenAI 兼容）、MCP `redact_text`/`redact_check` | 已完成 |
| **M4** | 50–100 份真实法律文书评测 + 规则调优 + 发布 | 持续 |
| **M5**（可选） | docx 字节级替换、PDF 文本层、图片涂黑 | 另议 |

---

## 13. 待定问题（需要维护者拍板）

1. **`scan` 默认是否打码？** 本稿建议“打码，`--reveal` 才显示全文”，避免终端泄露。
2. **`restore` 对 mask 风格的处理**：实现为“仅 placeholder 可还原；mask/black/hash 明确拒绝”，避免按位置误还原造成二次泄露。
3. **`date` / `amount` 默认关闭**：法律文书里日期/金额常需要保留语义；默认不脱，显式开启。
4. **ORG 无模型时怎么办**：v1 只支持 `deny`/`aliases`/自定义词典；自动识别 ORG 需要模型。
5. **模型运行时**：统一走 Ollama / OpenAI 兼容 Provider，不内置 `llama-server` 专用逻辑。检测到 provider 不可达时给出“启动 Ollama / 检查 base_url”的提示，不自动安装。
6. **占位符语言**：默认英文类型（`<PERSON_1>`），是否提供 `{type_cn}` 可选。

---

## 附录 A：第一轮评测数据

完整报告见 [`redact-benchmark.md`](redact-benchmark.md)，关键结论：

- 正则：确定性字段 P=1.00 / R=1.00；语义字段 0。
- HaS：核心 F1 0.992；ORG 100%、PERSON 100%、ADDRESS 100%；全角手机漏 1；同形词过脱 2。
- Horizon：F1 0.824；地址/手机/邮箱/护照满分；ORG 46%、ID 25%、USCC 40%。
- privyscope：F1 0.735；地址 100%、人名 57%、无 ORG。
- 正则 + HaS：F1 0.995，精度损失仅来自同形词。

## 附录 B：参考

- 元典脱敏：<https://tuomin.ailaw.cn/ydtm-site/docs.html>
- HaS 4.0：<https://huggingface.co/xuanwulab/HaS_4.0_0.6B>
- privyscope-zh-hans：<https://huggingface.co/zafrem/privyscope-zh-hans>
- Horizon pii-redactor-small：<https://huggingface.co/Horizon-Labs/pii-redactor-small>
- GB 11643（公民身份号码）、GB 32100（统一社会信用代码）
