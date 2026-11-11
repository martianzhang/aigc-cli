# 脱敏（redact）

`aigc-cli redact` 对**纯文本**做敏感信息识别、替换、还原和残留检查。

- **默认离线**：内置正则 + 校验位，不需要模型、不需要联网、不需要下载。
- **可复核**：识别与替换分离，`scan` 先看结果，`apply --spans` 再按复核结果替换。
- **可逆**：`style: placeholder` + 映射文件，可 `restore`。
- **可选模型**：通过 Ollama 或任意 OpenAI 兼容端点补充机构名、别名、地址等语义实体。
- **范围**：`txt / md / csv / json / stdin`。v1 不处理 Word、PDF、图片。

> 这是辅助工具，不能保证找到所有敏感信息，也不能替代人工复核或合规评估。

---

## 快速开始

```bash
# 只识别，不修改文件
aigc-cli redact scan 起诉状.txt

# 识别 + 替换（默认，不需要先 scan）
aigc-cli redact apply 起诉状.txt --mapping 起诉状.map.json

# 残留检查：用映射里的原文回扫输出
aigc-cli redact check 起诉状.redacted.txt --mapping 起诉状.map.json

# 还原
aigc-cli redact restore 起诉状.redacted.txt --mapping 起诉状.map.json

# 查看当前生效的规则
aigc-cli redact rules
```

- `-o/--output` 仍然是输出目录（和其他命令一致，默认当前目录）；`apply` 写 `<输出目录>/<原名>.redacted<扩展名>`，`restore` 写 `<输出目录>/<原名去掉 .redacted>.restored<扩展名>`，绝不覆盖输入。
- 需要管道时用 `--stdout`；stdin 输入默认也输出到 stdout。
- `check` 发现残留时退出码为 **2**，方便 CI 判断。

---

## 默认能识别什么

按类别（`redact rules` 会显示每条规则的 `category`）：

| 类别 | 默认启用 | 规则 |
|---|---|---|
| 个人信息 | ✅ | 身份证号（18/15 位；带“身份证号”标签或日期段合法的虚构号码也能识别）、手机、座机、400/800、邮箱、护照号（带“护照号”标签）、港澳台通行证/居住证、车架号/VIN、微信号、wxid、QQ、车牌、律师执业证号（带标签）、病历/就诊卡/住院号、工号、发动机/电机号、邮政编码、证件号；原告/被告/证人/法定代表人等角色词后的姓名、亲属关系后的姓名、英文姓名（Mr./Ms./Dr. 后）、民族/族别 |
| 个人信息 | ❌ | 裸护照号、裸 17 位律师证号（误伤高，建议配合标签用 `passport_ctx` / `lawyer_no_ctx`） |
| 单位与机构 | ✅ | 统一社会信用代码、工商注册号、项目编号/代号；常见中文公司/集团名（`...有限公司` / `...集团`）、`以下简称 XX 公司` 的简称 |
| 金融与网络 | ✅ | 银行卡号、银行账户/对公账户、JP 对公账户、MAC 地址、股票代码、发票代码/号码；密码/口令/API key/token、用户名/账号、数据库连接串、数据库主机、Authorization/Cookie 头、JWT、云厂商密钥、私钥块、带凭据 URL、加密钱包地址 |
| 金融与网络 | ✅ | IP、纳税人识别号、大小写金额（含中文大写）；URL 默认关闭 |
| 文书与日期 | ✅ | 司法案号、合同编号、公文文号、不动产权证书号、建设工程规划/施工/预售许可证号、保单号、著作权登记号；作品名（`《...》`，法条/司法解释名保留）；日期 |
| 文书与日期 | ❌ | 中文日期 |

**语义类**中的姓名会用角色词/亲属关系补召回；没有锚点的人名、机构别名、性别、国籍、民族、年龄、宗教信仰、生物识别、健康/疾病描述、境外法院、金额/日期仍不是正则能可靠解决的；凭据类只覆盖常见 `key=value` / `--key=value` / `key: value` / JSON 引号形式，**没有 key 或已知前缀的裸密钥无法可靠识别**，需要自定义规则、词表或模型：用 `dictionary` / `dictionary_files`，或 `engine: auto/model` 配合模型。日期/金额/民族默认脱敏（日期可反推身份证生日，金额可能包含薪酬等商业信息）。需要保留时设 `date: false` / `amount: false`，或 `--disable date_cn,amount,amount_cn,ethnicity_name,ethnicity_assign`；用 `--enable date_cn,amount,amount_cn,ethnicity_name,ethnicity_assign` 可强制重新开启。

---

## 配置

配置放在 `~/.config/aigc-cli/config.yaml` 的 `defaults.redact` 段。

### 引用外部配置文件

规则很多、或需要在不同案件/团队之间切换时，可以把 redact 配置放到独立 YAML，避免主配置越来越长。

`~/.config/aigc-cli/redact/strict.yaml`：

```yaml
style: mask
engine: auto
rules:
  phone_mobile: true
  bank_card: false
custom:
  - name: contract_no_custom
    type: CONTRACT_NO
    regex: '(?i)HT[-_]?[0-9]{4}[-_][0-9]{6}'
```

在 `defaults.redact` 中引用：

```yaml
defaults:
  redact:
    profile: strict              # 默认使用 strict
    profiles:
      strict: ./redact/strict.yaml
      loose:  ./redact/loose.yaml
    # 也可以只引用一个文件：
    # config_file: ./redact/default.yaml
```

- 配置里的相对路径按 **config.yaml 所在目录**解析，也支持 `~/`；命令行 `--redact-config` 的相对路径按当前目录解析；
- 外部文件是**覆盖层**：只写要改的字段即可；`rules` / `overrides` / `aliases` 等 map 按键合并，标量和列表替换；
- 外部文件可以是裸 redact 段，也可以包在 `redact:` 或 `defaults.redact:` 下；
- 优先级：内置默认 < `defaults.redact` < 外部文件/profile < `--rules-file` < 其他 CLI 参数；
- 单次切换不用改主配置：

```bash
aigc-cli redact scan 起诉状.txt --profile strict
aigc-cli redact apply 起诉状.txt --profile loose --mapping 起诉状.map.json
aigc-cli redact scan 起诉状.txt --redact-config ./redact/one-off.yaml
```

完整示例见 [`examples/redact/default.yaml`](examples/redact/default.yaml) 和
[`examples/redact/strict.yaml`](examples/redact/strict.yaml)。

`--rules-file` 仍然适合“只临时换规则三节（rules/overrides/custom）”的场景。

### 关闭/开启内置规则

只写你想改的，没写的保持默认：

```yaml
defaults:
  redact:
    rules:
      bank_card: false
      plate: true
```

也可以用命令改布尔项：

```bash
aigc-cli config set defaults.redact.rules.bank_card false
```

命令行临时覆盖：

```bash
aigc-cli redact scan file.txt --disable bank_card,plate
aigc-cli redact scan file.txt --enable passport,lawyer_no
```

`--enable` 在配置基础上追加启用，`--disable` 关闭；规则族会联动，例如
`--disable id_card` 会同时关掉 `id_card_15` / `id_card_ctx` / `id_card_loose`。

### 自定义规则

```yaml
defaults:
  redact:
    custom:
      - name: contract_no_custom
        type: CONTRACT_NO
        regex: '(?i)HT[-_]?[0-9]{4}[-_][0-9]{6}'
        style: mask
        validator: none
        boundary: word
        priority: 100
        max_len: 64
```

字段：

| 字段 | 说明 |
|---|---|
| `name` | 规则名（唯一；不能与内置重名） |
| `type` | 占位符/统计使用的类型，缺省为 `name` 大写 |
| `regex` | Go RE2 语法，**不支持 lookbehind/backreference** |
| `validator` | `none` / `id_card` / `uscc` / `luhn` / `phone` / `ip` / `url` |
| `boundary` | `none` / `digit` / `word`；只按 ASCII 邻接字符判断 |
| `group` | 只替换第 N 个捕获组（0 = 整个匹配） |
| `priority` | 重叠时优先级，越大越优先 |
| `max_len` | 最大字符数，防正则吞掉整段 |
| `enabled` | 默认 true |
| `mask` | 覆盖全局掩码策略 |

也可以用 `--rules-file rules.yaml` 临时替换 `rules` / `overrides` / `custom` 三节。

### 白名单 / 黑名单 / 别名

```yaml
defaults:
  redact:
    allow: ["北京市", "中国工商银行"]   # 命中也不脱
    deny:  ["某某项目"]                 # 命中必脱
    aliases:
      "北京云创智能科技有限公司": ["云创智能", "CloudGenius"]
      "张伟": ["老张"]
```

- `allow` 与 `deny` 不能有交集，否则启动时报错；
- `deny` 优先级高于 `allow`；
- 同一个别名组共用同一个占位符编号。

### 精确词表

没有模型时，可以用词表精确命中机构名、项目名、内部编号。小列表直接写在配置里：

```yaml
defaults:
  redact:
    dictionary:
      - value: "北京云创智能科技有限公司"
        type: ORG
```

大列表建议用外部文件（路径**相对于 config.yaml 所在目录**）：

```yaml
defaults:
  redact:
    dictionary_files:
      - "./dict/orgs.tsv"
      - "./dict/projects.json"
```

**推荐 TSV**：Excel 可直接导出，中文机构名/地址里常有逗号，TSV 不需要引号转义。也支持 CSV 与 JSON。

`dict/orgs.tsv`（`type` 可省略，默认 `SENSITIVE`；支持 `#` 注释和可选表头）：

```tsv
# 机构与项目
value	type
北京云创智能科技有限公司	ORG
某某项目	PROJECT
```

`dict/projects.json`：

```json
{"entries": [
  {"value": "合同HT-2024-1", "type": "CONTRACT_NO"}
]}
```

- 也支持 `[{"value": ...}]` 数组形式；
- 词表命中的优先级高于正则（不会被其他规则覆盖），并且会出现在 `scan` 结果里。

### 替换风格

```yaml
defaults:
  redact:
    style: placeholder        # placeholder | mask | black | hash
    placeholder: "<{type}_{n}>"
```

| 风格 | 效果 | 可还原 |
|---|---|---|
| `placeholder` | `<PERSON_1>`、`<PHONE_2>` | ✅ |
| `mask` | `138****5678` | ❌（按位置最佳努力） |
| `black` | `████` | ❌ |
| `hash` | `<PHONE_a1b2c3d4>` | ❌ |

### 掩码策略

```yaml
defaults:
  redact:
    style: mask
    mask:
      overrides:
        PHONE:      { position: middle, keep_prefix: 3, keep_suffix: 4 }
        ID_CARD:    { keep_prefix: 6, keep_suffix: 4 }
        BANK_CARD:  { position: prefix, keep_suffix: 4 }
        EMAIL:      { keep_prefix: 1, keep_suffix_regex: "@.*$" }
        PERSON:     { strategy: fixed, position: all, mask_len: 2 }  # 改为 **；默认张*
        ADDRESS:    { position: suffix, keep_prefix_regex: "^.*?(?:省|市|区|县|镇|街道)" }
```

- `position`：`middle`（保留首尾）/ `prefix`（只保留尾部）/ `suffix`（只保留头部）/ `all`；
- `strategy`：`fixed` / `name` / `email` / `address` / `phone` / `id_card` / `bank_card` / `date`；
- `preserve_length: false` + `mask_len: 3` 输出固定 `***`，隐藏原长度；
- `keep_separators: true` 保留空格、短横线，例如 `138-****-5678`；
- 姓名默认保留姓氏 + 固定一个 `*`（`张*` / `欧阳*`），不泄露名字长度；不想保留姓氏可显式配置 `PERSON: { strategy: fixed, position: all, mask_len: 2 }`；民族默认 `*族` / `█族`；
- 日期统一为 `****年**月**日`（或 `****-**-**`），数字全部遮盖，只保留年月日/分隔符，不泄露月日位数；black 风格为 `████年██月██日`；
- 内置类型自带合理默认；全局 `mask` 只作用于自定义/未知类型，避免偷偷削弱身份证等默认策略。

---

## 可选模型（Ollama）

模型是**第二层召回**，适合机构名、别名、地址、开集类型。默认不启用。

```bash
# 1. 用 Ollama 导入 HaS 4.0（或其他更强模型）
cat > Modelfile <<'EOF'
FROM ./has_4.0_0.6B_q4.gguf
EOF
ollama create has4 -f Modelfile
ollama serve
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

```bash
# 用 HaS / Ollama：模型名走 --model；--provider ollama 时 auto 会自动选 chatner
aigc-cli --no-proxy --provider ollama --model has4 redact scan 起诉状.txt --engine auto
# 也可以显式指定后端
aigc-cli --no-proxy --provider ollama --model has4 --model-engine chatner redact scan 起诉状.txt --engine auto

# 用本地 ONNX tokencls
aigc-cli --provider local --model-engine tokencls redact scan 起诉状.txt --engine auto
```

- `engine: rules`（默认）只跑正则；
- `engine: model` 只跑模型；
- `engine: auto` 规则 + 模型并集；
- `defaults.redact.model_engine`（或 `--model-engine`）选择模型后端：`auto` / `chatner` / `tokencls`；
- 模型名用 `--model has4`（或 `defaults.redact.model`），**不要**写进 `--model-engine`；
- 默认 `auto`：provider 是 `local` → tokencls；显式指定了非本地 provider（如 `ollama`）→ chatner；没有 provider 但配置了 tokencls → tokencls。
- 模型识别的 `ADDRESS` 会自动扩展到楼栋/单元/房间后缀（如 `3幢802室`），遇到标点或“联系电话/统一社会信用代码”等角色词停止；
- 值级模型返回的是实体字符串，不是 offset。默认 `occurrence: anchored`：只有实体附近出现“原告/联系人/电话/地址”等角色词时才替换，找不到锚点的出现位置会在 `scan` 的 `needs_review` 里列出，**不会自动替换**；
- 非 loopback 的模型端点默认拒绝，必须显式 `--allow-remote`，防止原文被误发到公网。

---

## 本地 ONNX 模型（可选）

`tokencls` 适配器直接加载本地 ONNX token-classification 模型，适合
privyscope / Horizon 这类带 offset 的模型。模型不随 CLI 二进制分发，但可以
直接从 `aigc-cli-models` 下载：

```bash
aigc-cli redact init --list
aigc-cli redact init --model zh-pii
```

默认安装到 `~/.config/aigc-cli/models/redact/zh-pii/`：

```
model.onnx        # ONNX 权重
config.json       # id2label 映射
tokenizer.json    # HuggingFace tokenizer
LICENSE.txt       # Apache-2.0
NOTICE.txt        # 上游来源与版本
```

- 模型：**privyscope-zh-hans 0.1.7**（简体中文 PII token classifier）；
- 上游：<https://github.com/zafrem/privyscope-zh-hans>；
- 许可：**Apache-2.0**；
- 下载镜像：`https://github.com/martianzhang/aigc-cli-models/releases/tag/v1`；
- 每个文件都带 SHA256 校验；已存在时跳过，损坏时提示，`--force` 可重新下载；
- 默认安装到 redact provider 的 `models_dir`；没有配置时用 `~/.config/aigc-cli/models`，`--models-dir` 可临时覆盖。

也可以手动把同样五个文件放进该目录。

```yaml
providers:
  local-redact:
    type: local
    models_dir: "~/.config/aigc-cli/models"

defaults:
  redact:
    provider: local-redact
    model: zh-pii
    model_engine: tokencls
    tokencls:
      model: zh-pii
      max_len: 256
      stride: 64
      operating_point: high_recall   # high_recall | balanced | high_precision
```

```bash
aigc-cli redact scan 起诉状.txt --engine model
```

- 模型输出可以是 BIO 或 BIOES；标签名通过 `CanonicalType` 归一（PERSON/ORG/ADDRESS/...）。
- `inputs` / `output` 一般不用写；适配器会先尝试 `input_ids + attention_mask + token_type_ids`，失败再退回两输入，并尝试 `logits` / `output` / `emissions`。
- `operating_point` 控制 BIOES 转移偏置：`high_recall`（默认，与 privyscope zh-Hans 一致）/ `balanced` / `high_precision`；也可以用 `biases: [outside, begin, single, inside, end, exit]` 六个值覆盖。
- 需要 ONNX Runtime；未安装时先跑任意本地命令的 `init`（例如 `aigc-cli ocr init`）。

## 如何验证模型是否生效

先准备一段**同时包含正则字段和语义字段**的文本：

```text
原告：张伟，电话13812345678。
被告：北京云创智能科技有限公司，住北京市朝阳区建国路88号。
联系人：高峰，电话13900001111。
备注：高峰时段的通行安排另行通知。
```

对比三档结果：

```bash
aigc-cli redact scan doc.txt --engine rules --json > rules.json
aigc-cli redact scan doc.txt --engine model --json > model.json
aigc-cli redact scan doc.txt --engine auto  --json > auto.json
```

预期：

| 引擎 | 结果 |
|---|---|
| `rules` | 只有手机号等正则字段；人名/机构/地址为空 |
| `model` | 只有模型识别的人名/机构/地址（不跑正则） |
| `auto` | 正则 + 模型并集，日常使用选它 |

然后检查：

1. `model.json` / `auto.json` 里有没有 `PERSON` / `ORG` / `ADDRESS`；
2. `needs_review` 是否包含“高峰时段”里的“高峰”（同形词不应自动替换）；
3. `apply` 输出里“高峰时段”是否保持原样；
4. `redact check` 是否发现残留。

模型路径分两种：

- **Ollama / OpenAI 兼容**：`defaults.redact.chatner`，按上面的 `providers.ollama` 配置；
- **本地 ONNX**：`defaults.redact.tokencls`，模型放 `models/redact/<model>/`。

自动测试：

```bash
go test ./internal/redact ./internal/cli/redact
AIGC_REDACT_ONNX_TEST=1 go test ./internal/redact -run TestTokenClassifierIntegration
```

## MCP 工具

MCP Server 额外提供两个只读工具：

- `redact_text`：离线规则 + 词表脱敏，返回脱敏文本；默认**不返回**原文，`include_mapping=true` 才返回含原文的映射。
- `redact_check`：传 `values` 列表或 `redact_text` 返回的 `mapping`，检查文本里的残留。

## 复核流程（可选）

```bash
aigc-cli redact scan 起诉状.txt --json > spans.json
# 人工编辑 spans.json：删误报、补漏报
aigc-cli redact apply 起诉状.txt --spans spans.json --mapping 起诉状.map.json
```

- `scan --json` 的 `start/end` 是**字符偏移**（rune），不是字节；
- `apply --spans` 会校验 `sha256`，文件被改过会拒绝（`--force` 可跳过）；
- `scan` 默认对预览值打码，`--reveal` 才显示原文；
- `scan --json` 里的 `text` 字段仍然是原文，请把该 JSON 当作敏感文件保管。

---

## 映射文件

`--mapping` 写出的 JSON 包含：

- `entities`：原文 ↔ 占位符；
- `occurrences`：输出文档中的位置；
- `source_sha256` / `output_sha256`：用于确认文档未被修改。

映射文件默认权限 `0600`。`--encrypt` 时会用 aigc-cli 主密钥加密，`restore` / `check` 时自动解密。

**映射文件本身是高敏文件**：持有它即可还原原文，请按组织规范保管。

---

## 限制

- 只处理纯文本；Word/PDF/图片/扫描件不在 v1 范围。
- 一个映射文件对应一个输出文档；跨文件共享编号（脱敏空间）尚未实现。
- `mask` / `black` / `hash` 不可逆；`restore` 只保证 `placeholder`。
- 日期、金额默认替换。设 `date: false` / `amount: false` 或用 `--disable date_cn,amount,amount_cn` 可保留；用 `--enable` 可重新开启。模型/自定义规则产生的 DATE/AMOUNT span 也受这两个开关控制。
- 不保证 100% 召回；导出前请用 `check` 回扫并人工复核。
- 不是合规认证或匿名化保证。
