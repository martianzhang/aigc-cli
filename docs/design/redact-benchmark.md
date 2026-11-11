# 中文法律文本脱敏：正则 / ONNX 模型 / HaS 对比（第一轮）

> 本文是 [`redact.md`](redact.md) 的评测附录。评测脚本是一次性原型，未入库；复现命令里的 `redact-bench/` 指临时工作目录。  
> 本轮 HaS 用 `llama-server` 跑；生产部署推荐用 Ollama 导入同一 GGUF（见设计稿 3.3），底层同为 llama.cpp，性能接近。

日期：2026-11-10  
语料：13 篇合成中文法律文书（起诉状、代理合同、租赁合同、劳动合同、NDA、律师函、执行申请、调解协议、股权转让、谅解书、授权委托书、证据清单、同形词通告），共 **4,428 字**，标注 **223 个实体**。

> 这是合成语料，格式比真实卷宗规整，绝对值会偏乐观；它用于比较引擎之间的**相对差异**和暴露工程问题，不作为最终准确率结论。

## 1. 总表

| 引擎 | 体积 | 冷启动 | 推理速度 | 核心 P | 核心 R | 核心 F1 | 依赖 |
|---|---|---|---|---|---|---|---|
| 纯正则 L0 | 0 | 0 | **4.4 ms / 4.4k 字**（≈100 万字/秒） | **1.000** | 0.419 | 0.591 | 无 |
| privyscope-zh-hans | **97.5 MB** | 0.61 s | 6,600 字/秒 | **1.000** | 0.581 | 0.735 | onnxruntime |
| Horizon pii-redactor-small（q8 + 校验后处理） | **268 MB** | 0.8 s | **4,000 字/秒** | 0.992 | 0.704 | 0.824 | onnxruntime |
| HaS 4.0 0.6B（Q4） | **378 MB**（FP16 1.2 GB） | ~1 s | **199 字/秒**（M4 + Metal） | 0.989 | **0.995** | **0.992** | llama-server |
| 正则 + Horizon | 268 MB | — | 4,000 字/秒 | 0.994 | 0.860 | 0.922 | onnxruntime |
| 正则 + HaS | 378 MB | — | 由 HaS 决定 | 0.989 | **1.000** | **0.995** | llama-server |
| 正则 + HaS + Horizon | 646 MB | — | 由 HaS 决定 | 0.984 | **1.000** | 0.992 | 两者 |

“核心”= PERSON / ORG / ADDRESS / PHONE / ID_CARD / BANK_CARD / EMAIL / USCC / CASE_NO / PLATE / PASSPORT / LAWYER_NO（不含 DATE / AMOUNT）。

## 2. 分类型（各引擎独立）

| 类型 | gold | 正则 R | privyscope R | Horizon R | HaS R | 说明 |
|---|---:|---:|---:|---:|---:|---|
| PERSON | 42 | 0.00 | 0.57 | 0.90 | **1.00** | privyscope 漏英文名、别名；Horizon 漏 4 个 |
| ORG | 41 | 0.00 | 0.00 | 0.46 | **1.00** | privyscope 无 ORG；Horizon 只中一半 |
| ADDRESS | 25 | 0.00 | **1.00** | **1.00** | **1.00** | 模型输出地址片段，需按类型合并 |
| PHONE | 25 | **1.00** | 0.88 | **1.00** | 0.96 | HaS 漏全角；privyscope 漏分隔符/全角 |
| ID_CARD | 12 | **1.00** | **1.00** | 0.25 | **1.00** | Horizon 在数字中间换标签，边界丢首字符 |
| BANK_CARD | 8 | **1.00** | 0.50 | 0.62 | **1.00** | privyscope 对 17/19 位卡号不稳 |
| EMAIL | 11 | **1.00** | **1.00** | **1.00** | **1.00** | |
| USCC | 10 | **1.00** | **1.00** | 0.40 | **1.00** | Horizon 把 USCC 标成 NATIONAL_ID/TAX_ID |
| CASE_NO | 5 | **1.00** | 0.00 | 0.00 | **1.00** | 只有 HaS 覆盖 |
| LAWYER_NO | 4 | **1.00** | 0.00 | 0.00 | **1.00** | 只有 HaS 覆盖 |
| PLATE | 2 | **1.00** | 0.00 | 0.00 | **1.00** | 只有 HaS 覆盖 |
| PASSPORT | 1 | **1.00** | 0.00 | **1.00** | **1.00** | |

## 3. 关键发现

### 3.1 纯正则把“确定性 PII”做到了 100%

在本语料上，手机、身份证、银行卡、邮箱、USCC、案号、车牌、护照、律师证号全部 **P=1.00 / R=1.00**，零误报。

前提是这三件事都做对了：

1. **校验位**：身份证 mod 11-2、USCC mod 31、银行卡 Luhn；只靠 `\d{18}` 会把流水号吞掉。
2. **边界**：Go RE2 没有 lookbehind，用“前后字符是不是 ASCII 字母数字”判断。**不能用 `unicode.IsLetter`**——中文紧邻数字时会被误判为词边界（本轮第一版就踩了这个坑）。
3. **全角折叠**：匹配前做 1:1 全角→半角折叠（offset 不变）；校验器也要先折叠，否则全角手机号会被判非法。

正则的短板完全在语义类：人名、机构、地址、日期、金额。

### 3.2 privyscope：地址强、人名偏弱、没有 ORG

- 优点：97.5 MB、6,600 字/秒、Apache-2.0、纯 ONNX，`EncodeCharOffsets` 直接拿 offset。
- 地址 100%，且会输出片段（`北京市朝阳区`），需要按类型做嵌套去重。
- 人名只有 57%：漏英文名、别名（老张/CloudGenius）、部分三字名。
- **标签体系固定 8 类，没有 ORG**；案号/车牌/护照/律师证号只能靠正则补。
- 银行卡 17/19 位不稳（本语料 50%）。

结论：适合做“轻量 ONNX 第一层”，但单独用不够。

### 3.3 HaS 4.0：召回几乎拉满，但有三个工程问题

**优点**

- ORG 100%、ADDRESS 100%、PERSON 100%（含英文名、别名、同一个人多处提及）。
- 结构化字段也几乎全中：银行卡、案号、车牌、护照、律师证号都能识别。
- 开集类型：给中文类型名就能识别，不依赖固定标签表。
- 4.0 版 Q4 只有 378 MB，M4 上 199 字/秒。

**问题**

1. **输出是实体值，不是 offset。** 我们只能全文回搜，于是“高峰”作为人名时，`高峰时段` 也被替换——本轮 2 个 FP 全来自这里。  
   这不是模型识别错，而是**缺少出现位置**导致的过脱。需要上下文锚点（联系人/姓名/原告）、人工复核，或让模型返回“第几次出现”。
2. **全角数字漏检。** `１３８...` 没有识别；正则补上即可。
3. **慢。** 199 字/秒意味着 1 万字约 50 秒；普通 x86 笔记本还会更慢。它适合做**兜底检查/短文档**，不适合做默认逐篇替换。
4. **日期偶发漏/合并。** 本轮漏 1 个日期；日期范围会返回成一个 span（可用一对多匹配处理）。

结论：**HaS 最适合做“第二遍 recall 兜底 + 开集类型补充”**，而不是唯一引擎。

### 3.4 Horizon：唯一“纯 ONNX 且带 ORG”的候选，但中文标识符不稳

- 268 MB q8、4,000 字/秒、无需 sidecar，体积和速度都比 HaS 好一个数量级。
- 地址、手机、邮箱、护照 **100%**；人名 **90%**；这是它最强的地方。
- 但机构只有 **46%**，身份证 **25%**，USCC **40%**，案号/车牌/律师证号 **0%**。
- 失败模式很典型：**在一个连续数字串中间换标签**（`NATIONAL_ID` → `TAX_ID`），并且会把首字符判成 `O` 导致边界丢一位。加“向连续字母数字扩展 + 校验位重新分类”的后处理后，精度从 0.978 提到 0.992，但召回仍是 0.704。
- 与正则并集：F1 **0.922**；与“正则 + HaS”的 0.995 差距明显。把 Horizon 再加进 HaS 组合反而多 1 个 FP（把保单号认成 USCC）。

**结论：如果坚持“纯 ONNX、不要 llama-server”，Horizon 是目前最好的选择；但它替代不了 HaS 的召回。**

### 3.5 组合效果

- 正则 + HaS：**R=1.000，F1=0.995**，唯一精度损失是同形词过脱。
- 再加 privyscope：**没有任何提升**（HaS 已覆盖它擅长的地址/人名）。

所以第一版不需要同时上两个神经网络模型。合理组合是：

```
L0 正则（默认，零模型）
   └─ 可选 L2：HaS scan（兜底，找 ORG/别名/语义实体）
        └─ 人工复核 + check 残留扫描
```

## 4. 建议的落地形态

1. `redact` 默认只跑 L0 正则：零下载、4ms、覆盖确定性 PII。
2. `--engine has` 作为可选兜底：调用本机 `llama-server`（OpenAI 兼容接口），只做 **scan**，不直接替换。
3. `redact check --engine has`：对已脱敏文本再扫一遍，发现残留就报警；这正是“兜底检查”的最佳位置。
4. 同形词问题用三层处理：角色词锚定（联系人/原告/被告 + 姓名）、白名单、人工复核。
5. privyscope 暂不作为默认依赖；如果后续需要一个 100MB 级、纯 ONNX、无 sidecar 的 NER，再引入。

## 5. 其他候选（调研中）

| 模型 | 体积 | 许可 | 备注 |
|---|---|---|---|
| **Horizon-Labs/pii-redactor-small** | ONNX q8 **268 MB** | Apache-2.0 | 已测：F1 0.824（正则并集 0.922）。地址/手机/邮箱/护照满分，人名 90%，但 ORG 46%、身份证 25%、USCC 40% |
| PII-Engineer-Chinese-NER-v1.0 | 408 MB | Apache-2.0 | GLiNER2，zh，开集；需 GLiNER 解码 |
| OpenMed-PII-Chinese-* | 278M–600M | Apache-2.0 | 中文 PII，但偏医疗域，且多数只有 safetensors |
| Mozilla smart-pii-redactor-v1 | 14.8 MB q4 | Apache-2.0 | 8 语言，中文未验证，标签偏英文聊天 |
| OpenAI privacy-filter | 1.5B | Apache-2.0 | 英文为主，体积过大 |
| RaNER / bert4ner | ~400 MB | Apache-2.0 | 通用新闻域，可补 ORG，但需 PyTorch/ModelScope |

## 6. 复现

```bash
cd redact-bench
source .venv/bin/activate
python corpus.py                    # 生成 corpus/corpus.jsonl
python regex_l0.py                  # out/regex_l0.jsonl
python run_privyscope.py            # out/privyscope.jsonl
llama-server -m models/has_4.0_0.6B_q4.gguf --port 8088 -ngl 99 -c 8192
python run_has.py                   # out/has.jsonl
python union.py out/regex_l0.jsonl out/has.jsonl
python score.py --engine union --file out/union.jsonl
```

## 7. 下一步

1. 用 50–100 份**真实**法律文书重跑（当前只有合成语料）。
2. 给 HaS 加“出现位置/上下文锚点”约束，解决同形词过脱；或只把它用于 `check`。
3. Horizon 需要“连续字母数字扩展 + 校验位重分类”的后处理，才能用于身份证/USCC；ORG 召回仍是硬伤。
4. 把 `mask` 策略（position / keep_prefix / keep_suffix / strategy）落到配置和单测。
5. 如果需要纯 ONNX 的 ORG，继续评估 PII-Engineer-Chinese / OpenMed 中文 PII。

## 参考项目与回归语料

除了自造样例，本轮规则评估参考了公开的法律文书脱敏项目，只提炼模式，不复制真实当事人数据：

- [qwertyzhu/legal-redactor](https://github.com/qwertyzhu/legal-redactor)（Apache-2.0）：虚构的起诉状、合同、判决书、律师函、判决叙事和负样例 corpus；
- [RayBagel/LegalMask](https://github.com/RayBagel/LegalMask)（MIT）：法条保护、案号、金额、日期、地址分层处理思路；
- [Lawbot 脱敏猫文档](https://www.lawbotai.cn/docs.html)：20 类法律文书敏感实体清单；
- 元典 / 律师企业法律顾问 AI 应用指引：客户名称、个人信息、商业秘密、金额/日期等脱敏分类。

据此补充的回归用例位于 `internal/redact/legal_corpus_test.go`，覆盖：

- 角色词后的姓名（原告/被告/证人/法定代表人/审判长）与亲属关系姓名；
- `以下简称 XX 公司` 的机构简称及全文传播；
- 虚构/校验位错误的身份证号（标签上下文 + 日期段宽松校验）；
- 地址楼栋/单元/房间后缀扩展；
- 作品名 `《...》` 与法条/司法解释名的区分；
- `账号` 后银行账号、IP、公司全称/简称、法条保护、负样例中的订单号。
