---
name: aigc-cli-midjourney
description: Use "aigc-cli midjourney" (alias "mj") to generate and edit images via Midjourney on APIMart. Supports imagine, blend, describe, edits, upscale, variation, reroll, zoom, pan, inpaint, modal, video, remix. All async task-based. Automatically polls task and downloads results.
---

# aigc-cli-midjourney

通过 `aigc-cli midjourney`（别名 `mj`）调用 APIMart Midjourney API 生成和编辑图片。所有操作均为异步任务模式，提交后自动轮询完成并下载结果。

## 前置条件

1. 项目已安装 `aigc-cli`（`go install` 或 `make build`）
2. 已配置 API Key（`~/.config/aigc-cli/config.yaml` 或 `APIMART_API_KEY` 环境变量）
   - MJ 默认参数在 `defaults.midjourney` 下配置（speed、version、style、size 等）

## 何时使用

- 用户需要 Midjourney 文生图（imagine）
- 用户需要图片融合（blend）、反推提示词（describe）、图片编辑（edits）
- 用户需要对已有结果进行放大（upscale）、变体（variation）、重绘（reroll）
- 用户需要扩图（zoom）、平移（pan）、局部重绘（inpaint → modal）
- 用户需要图生视频（video）
- 用户需要 v8/v8.1 重塑（remix-strong / remix-subtle）
- 用户需要 MJ 高级参数：character reference（cref）、style reference（sref）、depth reference（dref）、seed、stylize、chaos、weird、tile、stop 等
- 用户指定了 MJ 版本（v5.1/v6.1/v7/v8.1）或 niji 模式

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
aigc-cli mj imagine --prompt "a cute cat --ar 16:9"

# 结构化参数（推荐）
aigc-cli mj imagine \
  --prompt "a cute cat" \
  --size "16:9" \
  --version "6.1" \
  --style raw \
  --stylize 750

# 参考图
aigc-cli mj imagine \
  --prompt "turn into a luxury studio photo" \
  --image-url ./product.png \
  --iw 1.2

# Niji 二次元
aigc-cli mj imagine \
  --prompt "anime girl in a moonlit garden" \
  --niji --version "7" --size "9:16"

# v7 Draft 模式（快速出图，适合迭代）
aigc-cli mj imagine \
  --prompt "a cute cat" \
  --version "7" \
  --draft

# v8/v8.1 HD 模式（更高分辨率）
aigc-cli mj imagine \
  --prompt "a cute cat" \
  --version "8.1" \
  --hd

# 高级参数：cref（人物参考）、sref（风格参考）、seed（种子）
aigc-cli mj imagine \
  --prompt "a woman with red hair" \
  --cref https://example.com/face.png \
  --cw 80 \
  --sref https://example.com/style.png \
  --seed 12345
```

`imagine` 支持的所有结构化参数：

| 参数 | 说明 | 取值范围 |
|---|---|---|
| `--size` | 宽高比 | 如 `16:9`, `1:1`, `9:16` |
| `--version` | MJ 主版本 | `8.1`, `7`, `6.1`, `5.2`, `5.1` |
| `--style` | 风格覆盖 | 如 `raw` |
| `--quality` | 质量 | `0.25`, `0.5`, `1`, `2` |
| `--seed` | 种子（复现） | 整数 |
| `--stylize` / `--s` | 风格化程度 | 0-1000 |
| `--chaos` / `--c` | 混沌度 | 0-100 |
| `--weird` / `--w` | 怪异度 | 0-3000 |
| `--iw` | 图片权重 | 0-3 |
| `--cw` | 人物参考权重 | 0-100 |
| `--sw` | 风格权重 | 0-1000 |
| `--dw` | 深度权重 | 0-100 |
| `--cref` | 人物参考图 URL | |
| `--sref` | 风格参考图 URL | |
| `--dref` | 深度参考图 URL | |
| `--negative-prompt` | 负面提示词 | |
| `--tile` | 平铺模式 | 布尔 |
| `--niji` | Niji 模式 | 布尔 |
| `--raw` | Raw 风格（v5.1+） | 布尔 |
| `--draft` | 快速草稿（v7+） | 布尔 |
| `--hd` | 高清模式（v8/v8.1） | 布尔 |
| `--repeat` | 重复生成 | 2-40 |
| `--stop` | 提前停止 | 10-100 |
| `--extra` | 自定义 flag（追加到 prompt） | 如 `--extra "--no text"` |

### 2. Blend（多图融合）

```bash
aigc-cli mj blend --image-url a.png --image-url b.png --dimensions SQUARE
```

### 3. Describe（图片反推提示词）

```bash
aigc-cli mj describe --image-url input.png
```

### 4. Edits（图片编辑）

```bash
aigc-cli mj edits \
  --prompt "replace the background with a modern kitchen" \
  --image-url product.png
```

### 5. Upscale（放大）

```bash
# 常规放大（U1-U4，从现有图裁剪，毫秒级）
aigc-cli mj upscale --task-id task_xxx --index 1

# HD 高清放大（真实 2x 放大，60-120s）
# --index 仍指定哪张图，custom-id 告诉 API 用 HD upscale 而不是普通裁剪
# custom-id 可以从 query 结果中的 buttons 列表获取：
aigc-cli mj upscale --task-id task_xxx \
  --custom-id "MJ::JOB::upsample_v7_2x_subtle::1::abc"

# 流程示例：
# 1. imagine → 得到 task_a
# 2. upscale --task-id task_a --index 1 → 得到 task_b（常规）
# 3. 如果想 HD upscale：先 query task_a 看 buttons 中是否有 HD 选项
#    aigc-cli mj query task_a
#    然后取对应 custom-id 传给 upscale
```

### 6. Variation（变体）

```bash
aigc-cli mj variation --task-id task_xxx --index 3
aigc-cli mj high-variation --task-id task_xxx --index 2
aigc-cli mj low-variation --task-id task_xxx --index 4
```

### 7. Reroll / Zoom / Pan

```bash
aigc-cli mj reroll --task-id task_xxx
aigc-cli mj zoom --task-id task_xxx --zoom-ratio 1.5
aigc-cli mj pan --task-id task_xxx --direction right
```

### 8. Inpaint + Modal（局部重绘）

两步完成：先 inpaint 进入 MODAL 状态，再 modal 提交遮罩：

```bash
aigc-cli mj inpaint --task-id task_xxx
aigc-cli mj modal \
  --task-id task_yyy \
  --prompt "replace with red leather sofa" \
  --mask-url ./mask.png
```

### 9. Video（图生视频）

```bash
# 基本：图片 → 视频
aigc-cli mj video --image-url cat.png --motion high --batch-size 4

# 从 imagine 结果的一帧生成
aigc-cli mj video --task-id task_xxx --index 0 --animate-mode auto

# 指定 end frame 做 start/end 过渡
aigc-cli mj video --image-url start.png --end-url end.png --motion low

# 视频类型和分辨率
aigc-cli mj video --image-url cat.png --video-type "vid_1.1_i2v_720"
```

MJ Video 参数：

| 参数 | 说明 |
|---|---|
| `--image-url` | 首帧图片（必须或 --task-id 二选一） |
| `--task-id` | 复用 imagine 结果（必须或 --image-url 二选一） |
| `--index` | 搭配 --task-id，指定哪张图（0-3） |
| `--animate-mode` | `manual`（默认）或 `auto`（需 --task-id + --index） |
| `--motion` | `low` 或 `high`（默认） |
| `--batch-size` | 1 / 2 / 4（billed ×N） |
| `--video-type` | 分辨率：`vid_1.1_i2v_480`（默认）或 `vid_1.1_i2v_720` |
| `--end-url` | 尾帧 URL（启用 start/end 过渡动画） |

### 10. Remix（v8/v8.1 重塑）

```bash
# 强重塑（大幅改变构图/风格）
aigc-cli mj remix-strong --task-id task_xxx --index 1

# 弱重塑（保留主体/色调，小幅调整）
aigc-cli mj remix-subtle --task-id task_xxx --index 1 --prompt "new style"
```

### 11. 查询任务

```bash
aigc-cli mj query task_xxx
```

查询结果包含 `buttons` 列表，显示当前任务支持哪些后续操作（U1-U4、V1-V4、Zoom Out、Vary Region 等）。每个 button 的 `customId` 可直接传给 `--custom-id` 参数跳过自动匹配。

### 12. JSON 输入

```bash
aigc-cli mj imagine --json '{"prompt":"a cat","size":"16:9","version":"6.1"}'
```

### 13. Dry-run 调试

```bash
aigc-cli mj imagine --prompt "test" --dry-run
aigc-cli mj upscale --task-id task_xxx --index 1 --dry-run
```

### 14. 全部参数一览（所有子命令通用）

| 参数 | 适用子命令 | 说明 |
|---|---|---|
| `--prompt` / `-p` | imagine, edits, modal, video, remix | 文本提示词 |
| `--image-url` | imagine, blend, describe, edits, video | 图片 URL 或本地路径（可重复） |
| `--task-id` | upscale, variation, reroll, zoom, pan, inpaint, modal, video, remix | 父任务 ID |
| `--index` | upscale, variation, reroll, zoom, pan, video, remix | 网格中的图索引（1-4） |
| `--custom-id` | upscale, variation, reroll, zoom, pan | 直接指定 button（跳过自动匹配） |
| `--speed` | 全部 | relax / fast / turbo |
| `--dry-run` | 全部 | 打印 curl 不调用 API |
| `--json` | 全部 | JSON 文件/字符串/stdin |
| `-v` / `--verbose` | 全部 | 打印请求 JSON |

## 调试技巧

```bash
# 查看完整请求 JSON
aigc-cli mj imagine --prompt "test" -v

# Dry-run 查看 curl 命令（建议首次使用前先试）
aigc-cli mj imagine --prompt "test" --dry-run

# 查看任务全部 buttons（含 customId）
aigc-cli mj query task_xxx -v

# JSON 输入调试
echo '{"prompt":"a cat","size":"16:9"}' | aigc-cli mj imagine --json -
```

## 完整工作流示例

```bash
# 1. 文生图
aigc-cli mj imagine --prompt "a cute cat" --size "16:9" --version "6.1"

# 2. 选择第 1 张 upscale
aigc-cli mj upscale --task-id task_xxx --index 1

# 3. 对 upscale 后的图 zoom out 1.5x
aigc-cli mj zoom --task-id task_yyy --zoom-ratio 1.5

# 或 pan 向右
aigc-cli mj pan --task-id task_yyy --direction right

# 4. 或者对 upscale 后结果做局部重绘
aigc-cli mj inpaint --task-id task_yyy
aigc-cli mj modal --task-id task_zzz --prompt "add flowers" --mask-url ./mask.png
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
- Upscale（常规）从现有图裁剪，毫秒级返回；HD Upscale 需 60-120s
- Inpaint 进入 MODAL 后 30 分钟内必须调用 modal，否则自动取消退款
- Pan 仅 v6/v6.1/v7/niji 6 支持；Remix 仅 v8/v8.1 支持
- `--draft` 仅 v7+；`--hd` 仅 v8/v8.1
- 查询结果的 `buttons` 列表显示了当前任务支持的所有后续操作
- `--custom-id` 直接指定 button，跳过自动匹配（适用于 HD upscale、多版 Zoom Out 等）
- 首次使用某个子命令前，建议 `--dry-run` 确认请求参数
- 不要多次调用 API 重复测试，避免产生不必要的费用
