# Vision — 本地图像理解

> **Status**: Planning / 设计阶段
> **Branch**: `feat/vision`

基于 ONNX Runtime 在本地运行视觉模型，为纯文本 LLM 提供视觉能力。

## 设计约束

1. **模型 ≤ 1 GB** — 下载和运行成本可控
2. **走 ONNX Runtime** — 复用现有基础设施（`internal/onnxrt/`）
3. **视频 = 多帧图片** — ffmpeg 抽帧 + 逐帧描述 + 去重合并，不引入视频专用模型
4. **模型上传到 aigc-cli-models** — 和 OCR / AIGC / 去背一致

## 做什么 & 不做什么

### ✅ 做

- `aigc-cli vision init` — 下载视觉模型
- `aigc-cli vision describe` — 图片/视频描述
- Chat Agent / MCP 中自动调用 describe，结果注入纯文本 LLM
- 视频抽帧 + 逐帧描述 + 去重合并

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

### 目标检测（需要额外评估必要性）

| 模型 | 大小 | 许可证 | 输入 | 特点 |
|---|---|---|---|---|
| **YOLOv8n** | ~6 MB | AGPL-3.0 | 640×640 | 最快最小 |
| **YOLOv8s** | ~22 MB | AGPL-3.0 | 640×640 | 均衡 |
| **RF-DETR-base** | ~108 MB | Apache 2.0 | 可变 | 许可证友好 |
| **YOLOv11n** | ~5 MB | AGPL-3.0 | 640×640 | 最新版 |

### 是否需要检测？

目前 vision 的核心场景是"描述图片给 LLM 理解"，检测/分割不是必需。但如果后续需要"图里有什么物体/在哪里"，检测是基础能力。

## 需要评估的关键问题

在确定模型前，需要本机跑通验证：

### ONNX Runtime 兼容性

- [ ] `pure-onnx` 是否支持模型的全部算子（尤其是 Attention / MultiHeadAttention）
- [ ] 是否需要自定义 op 或 fallback
- [ ] 如果 `pure-onnx` 不支持，是否有替代方案

### 自回归推理

VLM 模型需要逐 token 生成，这是核心新增代码：

Florence-2 是 encoder-decoder 架构，KV Cache 管理比纯 decoder 复杂：

```
1. Encoder 一次性前向:
   图像 + 文本 prompt → 生成 cross-attention 所需的 KV

2. Decoder 自回归循环:
   输入: input_ids + kv_cache + encoder_kv
   输出: logits + 新的 kv_cache
   → sample → next_token
   → 拼入 output_ids
   → 更新 kv_cache (自注意力 + cross-attention)
   重复直到 EOS 或 max_len
```

建议抽象为接口：

```go
type VLModel interface {
    Encode(image []byte, prompt string) (embeddings, error)
    Step(inputIDs []int, kvCache *KVCache) (logits, *KVCache, error)
    Sample(logits) int
}
```

需要实现：
- `internal/vision/sampler.go` — greedy（默认）/ top-k / top-p
- `internal/vision/kvcache.go` — KV Cache 管理
- Tokenizer — ID 序列还原为文本

### Tokenizer

各模型的 tokenizer 不同：

| 模型 | Tokenizer | Go 实现可行性 |
|---|---|---|
| Florence-2 | BPE (GPT-2) | 中等 — 需要 BPE 实现 |
| Holo-3.1 | Qwen2 tokenizer | 较高 — 基于 SentencePiece |
| YOLO | 不需要 | 不适用 |
| Phi-3.5 | tiktoken | 较高 — 已有 Go 移植 |

### CPU 推理速度

目标：单帧描述 < 5s（CPU），否则 Chat 场景体验不可用。

| 模型 | ~350M | ~750M | ~800M |
|---|---|---|---|
| 估计速度 | 2-4s | 4-8s | 5-10s |

实际速度取决于模型架构、量化方式和自回归步数。

## 产品设计

```bash
# 下载模型（一次性）
aigc-cli vision init                    # 下载默认模型（Florence-2 INT8）
aigc-cli vision init --model fp16      # 可选其他版本

# 描述图片
aigc-cli vision describe photo.jpg
→ "一个穿着红色连衣裙的女孩在沙滩上奔跑"

# 描述视频
aigc-cli vision describe demo.mp4
→ "视频共 32 秒，关键画面：1) 一个人走进房间 2) 打开电脑..."

# Chat 自动看图
aigc-cli chat
> 这张图里有什么？
[attach image]
→ Agent 自动描述 → LLM 回复
```

## 视频理解

视频 = 多帧图片，与模型无关。

```
ffmpeg 抽帧 → for each: describe_image(frame) → 去重合并 → 输出
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
- [ ] 验证 `pure-onnx` 算子兼容性
- [ ] 实现 tokenizer + 采样循环原型
- [ ] 测量 CPU 推理速度
- [ ] 确定最终模型
- [ ] 上传到 aigc-cli-models

### P1 — 基础命令

- [ ] `internal/vision/` 推理引擎
- [ ] `cmd/vision.go`：`vision init` + `vision describe`
- [ ] 图片描述走通
- [ ] 视频抽帧 + 逐帧描述
- [ ] Chat Agent / MCP 工具注册

### P2 — Chat 自动触发 + 优化

- [ ] 图片/视频 attachment 自动 describe
- [ ] 多图、自定义 prompt
- [ ] 缓存、帧控制、流式

## 模型上传规范

确定模型后上传到 `martianzhang/aigc-cli-models` release v1：

```
vision_{model_name}.onnx
vision_{model_name}_tokenizer.json  # 如有
```

同时更新 aigc-cli-models README 的 License Attribution 表。

## 参考资料

- Florence-2: https://huggingface.co/microsoft/Florence-2-base
- Holo-3.1: https://huggingface.co/holomotion/Holo-3.1-0.8B-ONNX
- YOLOv8 ONNX: https://docs.ultralytics.com/modes/export
- pure-onnx: https://github.com/amikos-tech/pure-onnx
