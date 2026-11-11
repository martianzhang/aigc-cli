# Redaction (`redact`)

`aigc-cli redact` detects, replaces, restores and re-checks sensitive data in
**plain text**.

- **Offline by default**: built-in regex rules plus checksum validators. No
  model, no network, no download.
- **Reviewable**: detection and replacement are separate; `scan` previews,
  `apply --spans` applies reviewed spans.
- **Reversible**: `style: placeholder` plus a mapping file enables `restore`.
- **Optional model**: Ollama or any OpenAI-compatible endpoint adds
  organizations, aliases and addresses.
- **Scope**: `txt / md / csv / json / stdin`. Word, PDF and images are out of
  scope in v1.

> This is an assistant, not a compliance or anonymization guarantee. Always
> review the output before sharing it.

---

## Quick start

```bash
# Detect only, do not modify the file
aigc-cli redact scan complaint.txt

# Detect + replace (default; no prior scan needed)
aigc-cli redact apply complaint.txt --mapping complaint.map.json

# Check the output for leftover originals
aigc-cli redact check complaint.redacted.txt --mapping complaint.map.json

# Restore
aigc-cli redact restore complaint.redacted.txt --mapping complaint.map.json

# Show the effective rule set
aigc-cli redact rules
```

- `-o/--output` still means "output directory" (default: current directory). `apply` writes `<output-dir>/<stem>.redacted<ext>`; `restore` writes `<output-dir>/<stem-without-.redacted>.restored<ext>`. Neither ever overwrites the input.
- Use `--stdout` for pipelines; stdin input also defaults to stdout.
- `check` exits with code **2** when leftovers are found (useful in CI).

---

## What it detects by default

Grouped by category (`redact rules` shows each rule's `category`):

| Category | Default | Rules |
|---|---|---|
| Personal | ✅ | ID card (18/15), mobile, landline, 400/800, email, passport (labelled), HK/Macau/Taiwan permits/residence permit, VIN, WeChat ID, wxid, QQ, licence plate, lawyer licence (labelled), medical/visit/inpatient number, employee ID, engine/motor number, postcode, certificate number |
| Personal | ❌ | Bare passport, bare 17-digit lawyer licence (noisy; use the labelled `passport_ctx` / `lawyer_no_ctx` rules) |
| Organization | ✅ | Unified social credit code, business registration number |
| Finance & network | ✅ | Bank card, bank account/corporate account, JP corporate account, MAC address, stock code, invoice code/number |
| Finance & network | ❌ | IP, URL, amount |
| Documents & dates | ✅ | Case number, contract number, official document number, property certificate number, construction/planning/pre-sale permit number, policy number, copyright registration number |
| Documents & dates | ❌ | Chinese dates |

**Semantic entities** (names, addresses, organizations, gender, nationality,
ethnicity, age, religion, biometrics, health/condition descriptions, foreign
courts, amounts/dates) are not reliable regex problems. Use `dictionary` /
`dictionary_files`, or `engine: auto/model` with a model. Date/amount rules are
off by default; enable them with `--with-date` / `--with-amount` or config.

---

## Configuration

Redact settings live in the `defaults.redact` section of
`~/.config/aigc-cli/config.yaml`.

### External config files and profiles

When the rule set is large, or you switch between cases/teams, keep redact
settings in standalone YAML files instead of growing the main config.

`~/.config/aigc-cli/redact/strict.yaml`:

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

Reference it from `defaults.redact`:

```yaml
defaults:
  redact:
    profile: strict              # use strict by default
    profiles:
      strict: ./redact/strict.yaml
      loose:  ./redact/loose.yaml
    # Or reference a single file:
    # config_file: ./redact/default.yaml
```

- Relative paths inside the config resolve against the directory containing
  **config.yaml**; `~/` is also supported. A CLI `--redact-config` path is
  resolved from the current directory.
- The external file is an **overlay**: write only the fields you change. Maps
  such as `rules` / `overrides` / `aliases` merge key-by-key; scalars and lists
  replace the base value.
- The file may be a bare redact section, or wrapped in `redact:` /
  `defaults.redact:`.
- Precedence: built-ins < `defaults.redact` < external file/profile <
  `--rules-file` < other CLI flags.
- Switch for a single run without editing the main config:

```bash
aigc-cli redact scan complaint.txt --profile strict
aigc-cli redact apply complaint.txt --profile loose --mapping complaint.map.json
aigc-cli redact scan complaint.txt --redact-config ./redact/one-off.yaml
```

See [`examples/redact/default.yaml`](examples/redact/default.yaml) and
[`examples/redact/strict.yaml`](examples/redact/strict.yaml) for complete files.

`--rules-file` is still the lightweight option for temporarily replacing only
the rules/overrides/custom sections.


### Toggle built-in rules

Only write what you want to change:

```yaml
defaults:
  redact:
    rules:
      bank_card: false
      plate: true
```

Or from the CLI:

```bash
aigc-cli config set defaults.redact.rules.bank_card false
aigc-cli redact scan file.txt --disable bank_card,plate
aigc-cli redact scan file.txt --only id_card,phone_mobile
```

### Custom rules

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

| Field | Meaning |
|---|---|
| `name` | Unique rule name (must not collide with a built-in) |
| `type` | Type used for placeholders/statistics; defaults to upper-case name |
| `regex` | Go RE2 syntax; **no lookbehind/backreferences** |
| `validator` | `none` / `id_card` / `uscc` / `luhn` / `phone` / `ip` / `url` |
| `boundary` | `none` / `digit` / `word`; only ASCII neighbours are considered |
| `group` | Replace only capture group N (0 = whole match) |
| `priority` | Higher wins on overlap |
| `max_len` | Maximum characters, prevents runaway matches |
| `enabled` | Defaults to true |
| `mask` | Per-rule mask override |

`--rules-file rules.yaml` temporarily replaces the `rules`, `overrides` and
`custom` sections.

### Allow / deny / aliases

```yaml
defaults:
  redact:
    allow: ["北京市", "中国工商银行"]   # never redact
    deny:  ["某某项目"]                 # always redact
    aliases:
      "北京云创智能科技有限公司": ["云创智能", "CloudGenius"]
      "张伟": ["老张"]
```

- `allow` and `deny` must not overlap; startup fails if they do.
- `deny` wins over `allow`.
- Values in one alias group share a placeholder.

### Exact dictionary

Without a model, a dictionary matches organizations, project names and internal
codes exactly. Small lists can stay inline:

```yaml
defaults:
  redact:
    dictionary:
      - value: "北京云创智能科技有限公司"
        type: ORG
```

Larger lists belong in external files (paths are **relative to config.yaml**):

```yaml
defaults:
  redact:
    dictionary_files:
      - "./dict/orgs.tsv"
      - "./dict/projects.json"
```

**TSV is recommended**: it exports cleanly from Excel and avoids CSV quoting for
Chinese names containing commas. CSV and JSON are also supported.

`dict/orgs.tsv` (`type` is optional and defaults to `SENSITIVE`; `#` comments and
an optional header are supported):

```tsv
# organizations and projects
value	type
北京云创智能科技有限公司	ORG
某某项目	PROJECT
```

`dict/projects.json`:

```json
{"entries": [
  {"value": "合同HT-2024-1", "type": "CONTRACT_NO"}
]}
```

- A plain `[{"value": ...}]` array also works.
- Dictionary hits win over regex rules and appear in `scan` output.

### Replacement styles

```yaml
defaults:
  redact:
    style: placeholder        # placeholder | mask | black | hash
    placeholder: "<{type}_{n}>"
```

| Style | Output | Reversible |
|---|---|---|
| `placeholder` | `<PERSON_1>`, `<PHONE_2>` | ✅ |
| `mask` | `138****5678` | ❌ (positional best effort) |
| `black` | `████` | ❌ |
| `hash` | `<PHONE_a1b2c3d4>` | ❌ |

### Mask policy

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
        PERSON:     { position: suffix, keep_prefix: 1 }     # 张*
        ADDRESS:    { position: suffix, keep_prefix_regex: "^.*?(?:省|市|区|县|镇|街道)" }
```

- `position`: `middle` (keep both ends), `prefix` (keep the tail), `suffix`
  (keep the head), `all`.
- `strategy`: `fixed` / `name` / `email` / `address` / `phone` / `id_card` /
  `bank_card`.
- `preserve_length: false` with `mask_len: 3` hides the original length.
- `keep_separators: true` keeps spaces/dashes (`138-****-5678`).
- Built-in types ship with sensible defaults. The global `mask` section only
  applies to custom/unknown types, so a global `keep_prefix` cannot silently
  weaken the ID-card default.

---

## Optional model (Ollama)

The model is a **second recall layer** for organizations, aliases, addresses and
open-set types. It is off by default.

```bash
# Import HaS 4.0 (or any stronger model) into Ollama
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
# HaS / Ollama: the model name goes in --model; --provider ollama makes auto pick chatner
aigc-cli --no-proxy --provider ollama --model has4 redact scan complaint.txt --engine auto
# Or select the backend explicitly
aigc-cli --no-proxy --provider ollama --model has4 --model-engine chatner redact scan complaint.txt --engine auto

# Local ONNX tokencls
aigc-cli --provider local --model-engine tokencls redact scan complaint.txt --engine auto
```

- `engine: rules` (default): rules only.
- `engine: model`: model only.
- `engine: auto`: rules + model union.
- `defaults.redact.model_engine` (or `--model-engine`): `auto` / `chatner` / `tokencls`.
- The model name goes in `--model has4` (or `defaults.redact.model`), not in
  `--model-engine`.
- `auto` uses tokencls for a `local` provider, chatner for an explicit non-local
  provider (e.g. `ollama`), and tokencls when no provider is set but tokencls is
  configured.
- The model returns entity strings, not offsets. The default
  `occurrence: anchored` only replaces occurrences with a role word
  (原告/联系人/电话/地址…) nearby; ambiguous occurrences are listed under
  `needs_review` in `scan` and are **not** applied automatically.
- Non-loopback endpoints are rejected unless `--allow-remote` is passed.

---

## Local ONNX model (optional)

The `tokencls` adapter loads a local ONNX token-classification model
(privyscope / Horizon style). Models are not bundled into the binary, but can be
downloaded from `aigc-cli-models`:

```bash
aigc-cli redact init --list
aigc-cli redact init --model zh-pii
```

The default install directory is
`~/.config/aigc-cli/models/redact/zh-pii/`:

```
model.onnx        # ONNX weights
config.json       # id2label mapping
tokenizer.json    # HuggingFace tokenizer
LICENSE.txt       # Apache-2.0
NOTICE.txt        # upstream source and version
```

- Model: **privyscope-zh-hans 0.1.7** (Simplified Chinese PII token classifier).
- Upstream: <https://github.com/zafrem/privyscope-zh-hans>.
- License: **Apache-2.0**.
- Mirror: `https://github.com/martianzhang/aigc-cli-models/releases/tag/v1`.
- Every file is SHA256-verified; existing files are skipped, corrupt files are
  reported, and `--force` re-downloads.
- The default root is the redact provider's `models_dir`, falling back to
  `~/.config/aigc-cli/models`; `--models-dir` overrides it for one run.

Manual installation with the same five files works too.

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
aigc-cli redact scan complaint.txt --engine model
```

- BIO and BIOES labels are supported; labels are normalized to
  PERSON/ORG/ADDRESS/etc.
- `inputs` / `output` are usually unnecessary: the adapter tries
  `input_ids + attention_mask + token_type_ids` first, then two inputs, and
  tries `logits` / `output` / `emissions`.
- `operating_point` controls the BIOES transition biases: `high_recall`
  (default, matches privyscope zh-Hans), `balanced`, or `high_precision`. Pass
  six raw `biases: [outside, begin, single, inside, end, exit]` to override.
- ONNX Runtime must be installed; run any local command's `init` (e.g.
  `aigc-cli ocr init`) first.

## How to verify the model is working

Prepare a document that contains both regex-friendly and semantic data:

```text
原告：张伟，电话13812345678。
被告：北京云创智能科技有限公司，住北京市朝阳区建国路88号。
联系人：高峰，电话13900001111。
备注：高峰时段的通行安排另行通知。
```

Compare the three engines:

```bash
aigc-cli redact scan doc.txt --engine rules --json > rules.json
aigc-cli redact scan doc.txt --engine model --json > model.json
aigc-cli redact scan doc.txt --engine auto  --json > auto.json
```

Expected:

| Engine | Result |
|---|---|
| `rules` | Regex fields only; no names/orgs/addresses |
| `model` | Model spans only (rules are skipped) |
| `auto` | Rules + model union; use this in production |

Then check:

1. `model.json` / `auto.json` contains `PERSON` / `ORG` / `ADDRESS`;
2. `needs_review` contains the `高峰` from `高峰时段` (homonyms must not be auto-applied);
3. `apply` leaves `高峰时段` untouched;
4. `redact check` reports any leftovers.

Two model backends:

- **Ollama / OpenAI-compatible**: `defaults.redact.chatner`, configured through `providers.ollama`;
- **Local ONNX**: `defaults.redact.tokencls`, model files under `models/redact/<model>/`.

Automated tests:

```bash
go test ./internal/redact ./internal/cli/redact
AIGC_REDACT_ONNX_TEST=1 go test ./internal/redact -run TestTokenClassifierIntegration
```

## MCP tools

The MCP server exposes two read-only tools:

- `redact_text`: offline rules + dictionary redaction; originals are omitted by
  default, set `include_mapping=true` to include the reversible mapping.
- `redact_check`: pass a `values` list or a `mapping` from `redact_text` to find
  leftover originals.

## Review workflow (optional)

```bash
aigc-cli redact scan complaint.txt --json > spans.json
# Edit spans.json: remove false positives, add missed spans
aigc-cli redact apply complaint.txt --spans spans.json --mapping complaint.map.json
```

- `scan --json` uses **rune offsets** (characters), not bytes.
- `apply --spans` verifies `sha256`; use `--force` to override.
- `scan` masks values in the human table by default; `--reveal` shows originals.
- The `text` fields in `scan --json` are still originals; treat that JSON as sensitive.

---

## Mapping file

`--mapping` writes a JSON file with:

- `entities`: original value ↔ placeholder;
- `occurrences`: positions in the redacted output;
- `source_sha256` / `output_sha256`: document integrity.

The file is written with mode `0600`. `--encrypt` encrypts it with the aigc-cli
master key; `restore` and `check` decrypt it automatically.

**The mapping is as sensitive as the original document.** Anyone holding it can
restore the text.

---

## Limitations

- Plain text only; Word/PDF/images/scans are out of scope in v1.
- One mapping file describes one output document; cross-file shared numbering
  (a "redaction space") is not implemented yet.
- `mask` / `black` / `hash` are one-way; only `placeholder` is reliably
  reversible.
- Dates and amounts are not replaced by default; use `--with-date` /
  `--with-amount` (or `date: true` / `amount: true`).
- Recall is not 100%; run `check` and review manually before sharing.
- Not a compliance or anonymization guarantee.
