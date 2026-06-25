---
name: apimart-midjourney
description: Use "apimart-cli midjourney" (alias "mj") to generate and edit images via Midjourney on APIMart. Supports imagine, blend, describe, edits, upscale, variation, reroll, zoom, pan, inpaint, modal, video, remix. All async task-based. Automatically polls task and downloads results.
---

# apimart-midjourney

通过 `apimart-cli midjourney`（别名 `mj`）调用 APIMart Midjourney API 生成和编辑图片。所有操作均为异步任务模式，提交后自动轮询完成并下载结果。

## 前置条件

1. 项目已安装 `apimart-cli`（`go install` 或 `make build`）
2. 已配置 API Key（`~/.config/apimart/config.yaml` 或 `APIMART_API_KEY` 环境变量）
   - MJ 默认参数在 `defaults.midjourney` 下配置（speed、version、style、size 等）

## 何时使用

- 用户需要 Midjourney 文生图（imagine）
- 用户需要图片融合（blend）、反推提示词（describe）、图片编辑（edits）
- 用户需要对已有结果进行放大（upscale）、变体（variation）、重绘（reroll）
- 用户需要扩图（zoom）、平移（pan）、局部重绘（inpaint → modal）
- 用户需要图生视频（video）
- 用户需要 v8/v8.1 重塑（remix-strong / remix-subtle）

## 工作流

```
imagine → upscale → zoom / pan / inpaint → modal
  ↓         ↓
reroll    variation / high-variation / low-variation
```

## 使用流程

### 1. Imagine（文生图 / 图生图）

```bash
# 基本文生图（MJ 原生 flag 可直接写 prompt 里）
apimart-cli mj imagine --prompt "a cute cat --ar 16:9"

# 结构化参数（推荐）
apimart-cli mj imagine \
  --prompt "a cute cat" \
  --size "16:9" \
  --version "6.1" \
  --style raw \
  --stylize 750

# 参考图
apimart-cli mj imagine \
  --prompt "turn into a luxury studio photo" \
  --image-url ./product.png \
  --iw 1.2

# Niji 二次元
apimart-cli mj imagine \
  --prompt "anime girl in a moonlit garden" \
  --niji --version "7" --size "9:16"
```

### 2. Blend（多图融合）

```bash
apimart-cli mj blend --image-url a.png --image-url b.png --dimensions SQUARE
```

### 3. Describe（图片反推提示词）

```bash
apimart-cli mj describe --image-url input.png
```

### 4. Edits（图片编辑）

```bash
apimart-cli mj edits \
  --prompt "replace the background with a modern kitchen" \
  --image-url product.png
```

### 5. Upscale（放大）

```bash
# 常规放大（U1-U4，从现有图裁剪，毫秒级）
apimart-cli mj upscale --task-id task_xxx --index 1

# HD 高清放大（真实 2x 放大，60-120s）
apimart-cli mj upscale --task-id task_xxx \
  --custom-id "MJ::JOB::upsample_v7_2x_subtle::1::abc"
```

### 6. Variation（变体）

```bash
apimart-cli mj variation --task-id task_xxx --index 3
apimart-cli mj high-variation --task-id task_xxx --index 2
apimart-cli mj low-variation --task-id task_xxx --index 4
```

### 7. Reroll / Zoom / Pan

```bash
apimart-cli mj reroll --task-id task_xxx
apimart-cli mj zoom --task-id task_xxx --zoom-ratio 1.5
apimart-cli mj pan --task-id task_xxx --direction right
```

### 8. Inpaint + Modal（局部重绘）

两步完成：先 inpaint 进入 MODAL 状态，再 modal 提交遮罩：

```bash
apimart-cli mj inpaint --task-id task_xxx
apimart-cli mj modal \
  --task-id task_yyy \
  --prompt "replace with red leather sofa" \
  --mask-url ./mask.png
```

### 9. Video（图生视频）

```bash
apimart-cli mj video --image-url cat.png --motion high --batch-size 4
```

### 10. Remix（v8/v8.1 重塑）

```bash
apimart-cli mj remix-strong --task-id task_xxx --index 1
apimart-cli mj remix-subtle --task-id task_xxx --index 1 --prompt "new style"
```

### 11. 查询任务

```bash
apimart-cli mj query task_xxx
```

查询结果包含 `buttons` 列表，显示当前任务支持哪些后续操作（U1-U4、V1-V4、Zoom Out、Vary Region 等）。每个 button 的 `customId` 可直接传给 `--custom-id` 参数跳过自动匹配。

### 12. JSON 输入

```bash
apimart-cli mj imagine --json '{"prompt":"a cat","size":"16:9","version":"6.1"}'
```

### 13. Dry-run 调试

```bash
apimart-cli mj imagine --prompt "test" --dry-run
apimart-cli mj upscale --task-id task_xxx --index 1 --dry-run
```

## 配置默认值

```yaml
defaults:
  midjourney:
    speed: fast        # relax / fast / turbo
    version: "6.1"     # MJ 版本
    style: raw
    size: "16:9"
```

## 注意事项

- MJ 为异步任务模型，提交后自动轮询，最长等待 180 秒
- Upscale 从现有图裁剪，毫秒级返回；HD Upscale 需 60-120s
- Inpaint 进入 MODAL 后 30 分钟内必须调用 modal，否则自动取消退款
- Pan 仅 v6/v6.1/v7/niji 6 支持；Remix 仅 v8/v8.1 支持
- 查询结果的 `buttons` 列表显示了当前任务支持的所有后续操作
