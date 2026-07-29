# Vision — 本地图像理解

> **Status**: In Progress / 实现中
> **Branch**: `feat/vision`

基于 ONNX Runtime 在本地运行视觉模型，为纯文本 LLM 提供视觉能力。

## 设计约束

1. **模型 ≤ 1 GB** — 下载和运行成本可控
2. **走 ONNX Runtime** — 复用现有基础设施（`internal/onnxrt/`）
3. **视频 = 多帧图片** — ffmpeg 抽帧 + 逐帧推理 + 去重合并，不引入视频专用模型
4. **模型上传到 aigc-cli-models** — 和 OCR / AIGC / 去背一致
5. **Describe 和 Ask 共享同一推理引擎** — `ask` 只是换一个任务提示词

## 做什么 & 不做什么

### ✅ 做

- `aigc-cli vision init` — 下载视觉模型
- `aigc-cli vision describe` — 图片/视频描述（默认 `DETAILED_CAPTION`）
- `aigc-cli vision describe --ask "question"` — 基于图片提问（VQA）
- Chat Agent / MCP 中自动调用 `describe` / `describe --ask`
- 视频抽帧 + 逐帧推理 + 去重合并

### ❌ 不做

- 不做 Ollama 集成
- 不依赖外部 API
- 不做目标检测/分割（除非有明确需求）

## 候选模型

以下模型均在 ≤ 1GB 约束内，按类型分类评估：

### 视觉语言模型（图像描述 / VQA）

| 模型 | 大小 | 许可证 | 输入 | 特点 |
|---|---|---|---|---|
| **Florence-2-base INT8** | **~270 MB** ✅ | MIT | 224×224 | 强烈推荐：最小、CPU 1-3s |
| **Florence-2-base FP16** | ~520 MB | MIT | 224×224 | GPU 可用时备选 |
| **Florence-2-large** | ~750 MB | MIT | 224×224 | 精度更高 |
| **Holo-3.1-0.8B** | ⚠️ 待确认 (HF 页面 3.04GB) | Apache 2.0 | 384×384 | 实际 ONNX 可能 >1GB |
| **Phi-3.5-vision (INT4)** | ~2.5 GB ❌ | MIT | 224×224 | 超约束 |
| **Qwen2-VL (INT4)** | ~1.8 GB ❌ | Apache 2.0 | 可变 | 超约束 |
| **PaliGemma-3B (INT4)** | ~1.7 GB ❌ | Apache 2.0 | 224×224 | 超约束 |

≤ 1GB 且经过实测验证的选择只有 **Florence-2 INT8/FP16**。Holo-3.1 的 ONNX 版本 HF 仓库总显示 3.04GB，需要确认 CPU 推理目录实际是否 ≤745MB。

**关键优势：Florence-2 原生支持 VQA**，通过 `<VQA>` 任务 token + 用户问题即可实现 `--ask`，无需额外模型。

### 目标检测（需要额外评估必要性）

| 模型 | 大小 | 许可证 | 输入 | 特点 |
|---|---|---|---|---|
| **YOLOv8n** | ~6 MB | AGPL-3.0 | 640×640 | 最快最小 |
| **YOLOv8s** | ~22 MB | AGPL-3.0 | 640×640 | 均衡 |
| **RF-DETR-base** | ~108 MB | Apache 2.0 | 可变 | 许可证友好 |
| **YOLOv11n** | ~5 MB | AGPL-3.0 | 640×640 | 最新版 |

### 是否需要检测？

目前 vision 的核心场景是"描述图片给 LLM 理解"和"对图片提问"，检测/分割不是必需。但如果后续需要"图里有什么物体/在哪里"，检测是基础能力。

## 底层架构

### Describe 与 Ask 共享同一引擎

```
                    ┌───────────────────────────────────┐
                    │          Vision Engine             │
                    │  (internal/vision/engine.go)       │
                    │                                   │
                    │  input: image + task_prompt        │
                    │  output: generated text            │
                    └───────────────────────────────────┘
                               ▲
              ┌────────────────┴────────────────┐
              │                                 │
    ┌─────────┴──────────┐          ┌──────────┴──────────┐
    │  describe           │          │  describe --ask     │
    │  prompt:            │          │  prompt:            │
    │  "<DETAILED_CAPTION>"│         │  "<VQA> question"   │
    └────────────────────┘          └─────────────────────┘
```

**代码层面**只是一行分支：

```go
if askQuestion != "" {
    prompt = fmt.Sprintf("<VQA> %s", askQuestion)
} else {
    prompt = "<DETAILED_CAPTION>"
}
```

Florence‑2 官方任务清单本来就有 `<VQA>`（视觉问答），`ask` 只是换一个任务提示词，推理引擎完全复用。

### 推理流程

```
┌───────────────────────────────────────────┐
│  1. 预处理                                 │
│     image(224×224) → normalize → tensor    │
│     prompt → tokenize → input_ids          │
└───────────────────────────────────────────┘
                      │
                      ▼
┌───────────────────────────────────────────┐
│  2. Encoder 一次性前向                     │
│     pixel_values + input_ids               │
│     → encoder_hidden_states (cross-attn KV)│
└───────────────────────────────────────────┘
                      │
                      ▼
┌───────────────────────────────────────────┐
│  3. Decoder 自回归循环                      │
│     input: decoder_input_ids                │
│          + encoder_hidden_states            │
│          + past_key_values (KV cache)       │
│     output: logits + new past_key_values    │
│     → sample → next_token                  │
│     → 拼入 output_ids                      │
│     → 更新 KV cache                        │
│     重复直到 EOS 或 max_len                │
└───────────────────────────────────────────┘
                      │
                      ▼
┌───────────────────────────────────────────┐
│  4. 后处理                                 │
│     output_ids → detokenize → text         │
└───────────────────────────────────────────┘
```

### 视频场景的处理

视频走 `--ask` 时（或 `describe`），逻辑是：
1. ffmpeg 抽帧（按时长自适应帧数）
2. **每一帧走同一推理流程**（同一问题或同一描述任务）
3. 汇总所有帧的答案 → 去重合并

示例输出：
```
视频共 32 秒，抽取 16 帧分析"是否有人拿手机"：
- 第 1-8 帧：无人拿手机
- 第 9-16 帧：一个穿蓝衣服的人拿着手机
结论：视频后半段有人拿手机
```

### 接口抽象

```go
// VLModel 是视觉语言模型的推理接口。
// Describe 和 Ask 共用同一接口，只是 prompt 不同。
type VLModel interface {
    // Encode 对图像和文本 prompt 执行 encoder 前向，返回 encoder hidden states。
    Encode(pixels []float32, inputIDs []int64) ([]float32, error)

    // Step 执行 decoder 单步推理，返回 logits 和更新后的 KV cache。
    Step(inputID int64, encoderStates []float32, kvCache *KVCache) ([]float32, *KVCache, error)

    // Sample 从 logits 中采样下一个 token ID。
    Sample(logits []float32) int64
}
```

## 产品设计

### 命令

```bash
# 下载模型（一次性）
aigc-cli vision init                       # 下载默认模型（Florence-2 INT8）
aigc-cli vision init --model base-fp16    # 可选其他版本
aigc-cli vision init --list                # 列出可用模型

# 描述图片
aigc-cli vision describe photo.jpg
→ "一个穿着红色连衣裙的女孩在沙滩上奔跑"

# 基于图片提问（VQA）
aigc-cli vision describe photo.jpg --ask "What color is the car?"
→ "The car in the image is red."

# 视频 + 提问（每帧都问同一个问题）
aigc-cli vision describe demo.mp4 --ask "Is the person holding a phone?"
→ "视频共 32 秒，抽取 16 帧分析：..."

# Chat 自动看图
aigc-cli chat
> 这张图里有什么？
[attach image]
→ Agent 自动 ask → LLM 回复
```

### 参数设计

| 参数 | 适用命令 | 说明 |
|---|---|---|
| `--ask` / `-a` | `describe` | 启用 VQA 模式，后接问题文本 |
| `--model` | `describe` | 指定模型变体（base-int8 / base-fp16 / large） |
| `--max-tokens` | `describe` | 最大生成 token 数（默认 512） |
| `--temperature` | `describe` | 采样温度（默认 0.7） |
| `--top-k` | `describe` | Top-K 采样（默认 40） |
| `--frames` | 视频输入 | 手动指定抽帧数（默认自动） |
| `--verbose` | 全局 | 显示推理耗时、token 数等 |

### 命令树

```
aigc-cli vision
├── init         下载模型（--list 列出可用模型）
└── describe     描述图片/视频（--ask 进入 VQA 模式）
```

## 视频理解

视频 = 多帧图片，与模型无关。

```
ffmpeg 抽帧 → for each: infer(frame, prompt) → 去重合并 → 输出
```

### ffmpeg 依赖

```bash
# 未安装时提示：
macOS:  brew install ffmpeg
Linux:  apt install ffmpeg
Windows: winget install ffmpeg
```

### 帧采样

| 时长 | 帧数 | 策略 |
|---|---|---|
| ≤ 1 分钟 | 30 帧 | 均匀 0.5fps |
| 1-5 分钟 | 60 帧 | 均匀 |
| > 5 分钟 | 60 帧 | 降采样 |

## 实现计划

### P0 — 模型评估与选择

- [ ] 选定 1-2 个候选模型，本地跑通 ONNX 推理
- [ ] 验证 `pure-onnx` 算子兼容性（尤其是 Attention / MultiHeadAttention）
- [ ] 实现 tokenizer + 采样循环原型
- [ ] 测量 CPU 推理速度
- [ ] 确定最终模型
- [ ] 上传到 aigc-cli-models（含 encoder + decoder ONNX + vocab/merges）

### P1 — 基础命令（含 --ask）

- [ ] `internal/vision/` 推理引擎（共享 Describe + Ask）
  - [ ] Tokenizer（GPT-2 BPE）
  - [ ] 图像预处理（224×224 resize + normalize）
  - [ ] KV Cache 管理
  - [ ] Sampler（greedy）
  - [ ] ONNX 推理循环（encoder → decoder autoregressive）
  - [ ] Engine 接口：Describe() / Ask() 共享底层
- [ ] `cmd/vision.go`：
  - [ ] `vision init` — 下载模型
  - [ ] `vision describe` — 默认 DETAILED_CAPTION
  - [ ] `vision describe --ask "..."` — VQA
- [ ] 图片推理走通（describe + ask）
- [ ] 视频抽帧 + 逐帧推理
- [ ] Chat Agent / MCP 工具注册

### P2 — Chat 自动触发 + 优化

- [ ] 图片/视频 attachment 自动 describe 或 ask
- [ ] 多图、自定义 prompt
- [ ] 缓存、帧控制、流式

## 模型上传规范

确定模型后上传到 `martianzhang/aigc-cli-models` release v1：

```
vision_encoder_{variant}.onnx
vision_decoder_{variant}.onnx
vision_{variant}_vocab.json       # GPT-2 BPE vocabulary
vision_{variant}_merges.txt       # BPE merge rules
```

Florence-2 base INT8 示例：

```
vision_encoder_base-int8.onnx    # ~150 MB
vision_decoder_base-int8.onnx    # ~120 MB
vision_base-int8_vocab.json      # ~1 MB
vision_base-int8_merges.txt      # ~0.5 MB
```

同时更新 aigc-cli-models README 的 License Attribution 表。

## 参考资料

- Florence-2: https://huggingface.co/microsoft/Florence-2-base
- Florence-2 ONNX export: https://huggingface.co/microsoft/Florence-2-base/tree/main/onnx
- Holo-3.1: https://huggingface.co/holomotion/Holo-3.1-0.8B-ONNX
- YOLOv8 ONNX: https://docs.ultralytics.com/modes/export
- pure-onnx: https://github.com/amikos-tech/pure-onnx
