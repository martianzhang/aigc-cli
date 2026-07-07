# Midjourney 图片生成

Midjourney 使用**异步任务模型** — 提交任务 → 获取 `task_id` → 轮询结果。所有 MJ 端点都在 `/v1/midjourney/` 下。

命令别名为 `mj`，两种写法等价：
```bash
aigc-cli midjourney imagine --prompt "..."
aigc-cli mj imagine --prompt "..."
```

## 工作流概览

```
imagine → upscale → zoom / pan / inpaint → modal
  ↓         ↓
reroll    variation / high-variation / low-variation
```

1. **imagine** — 生成 2x2 网格（4 张图）
2. **upscale** — 选一张放大 → 得到单张高清图
3. 单张图后可继续 **zoom**（扩图）/ **pan**（平移）/ **inpaint**（局部重绘→modal）

---

## Imagine（文生图 / 图生图）

```bash
# 基本文生图
aigc-cli mj imagine --prompt "a cute cat --ar 16:9"

# 结构化参数（推荐，body 值会覆盖 prompt 中的同名 flag）
aigc-cli mj imagine \
  --prompt "a cute cat" \
  --size "16:9" \
  --version "6.1" \
  --style raw \
  --stylize 750

# 参考图（图生图）
aigc-cli mj imagine \
  --prompt "turn into a luxury studio photo" \
  --image-url ./product.png \
  --iw 1.2

# Fast 模式
aigc-cli mj imagine --prompt "a cute cat" --speed fast

# Niji 二次元
aigc-cli mj imagine \
  --prompt "anime girl in a moonlit garden" \
  --niji --version "7" --size "9:16"

# JSON 输入
aigc-cli mj imagine --json '{"prompt":"a cat","size":"16:9","version":"6.1"}'
```

### Imagine 参数

| 参数 | 说明 |
|---|---|
| `--prompt` / `-p` | 提示词；MJ 原生 flag 可直接写在 prompt 里（如 `--ar 16:9`） |
| `--image-url` | 参考图 URL 或本地路径（可重复） |
| `--speed` | `relax`（默认） / `fast` / `turbo` |
| `--size` | 宽高比，如 `16:9`、`1:1`、`9:16` → 对应 `--ar` |
| `--quality` | 质量：`0.25`、`0.5`、`1`、`2` → 对应 `--q` |
| `--style` | 风格覆盖，如 `raw` → 对应 `--style` |
| `--version` | MJ 版本：`8.1`、`7`、`6.1`、`5.2`、`5.1` → 对应 `--v` |
| `--seed` | 随机种子 → 对应 `--seed` |
| `--negative-prompt` | 负面提示词 → 对应 `--no` |
| `--stylize` | 风格化 0-1000 → 对应 `--s` |
| `--chaos` | 混乱度 0-100 → 对应 `--c` |
| `--weird` | 怪异度 0-3000 → 对应 `--w` |
| `--tile` | 平铺模式 → 对应 `--tile` |
| `--niji` | Niji 模型开关 |
| `--iw` | 图片权重 0-3 → 对应 `--iw` |
| `--cw` | 角色参考权重 0-100 → 对应 `--cw` |
| `--sw` | 风格权重 0-1000 → 对应 `--sw` |
| `--cref` | 角色参考图 URL → 对应 `--cref` |
| `--sref` | 风格参考图 URL → 对应 `--sref` |
| `--dref` | 深度参考图 URL → 对应 `--dref` |
| `--dw` | 深度权重 0-100 → 对应 `--dw` |
| `--repeat` | 重复次数 2-40 → 对应 `--repeat` |
| `--raw` | Raw 风格（v5.1+） |
| `--draft` | 草稿模式（v7+） |
| `--hd` | HD 模式（v8/v8.1） |
| `--stop` | 提前停止 10-100 |
| `--extra` | 额外 flag 转义口，原样追加到 prompt |
| `--json` | JSON 输入 |
| `--dry-run` | 打印 curl 不调用 API |

---

## Blend（多图融合）

融合 2-4 张图，纯图片操作，**不接受 prompt**：

```bash
# 基础融合
aigc-cli mj blend --image-url a.png --image-url b.png

# 指定比例
aigc-cli mj blend \
  --image-url a.png --image-url b.png --image-url c.png \
  --dimensions PORTRAIT

# 自由比例（优先级高于 --dimensions）
aigc-cli mj blend \
  --image-url a.png --image-url b.png \
  --size "16:9"
```

| 参数 | 说明 |
|---|---|
| `--image-url` | 图片 URL/路径（必填，2-4 个） |
| `--dimensions` | 比例预设：`SQUARE`(1:1)、`PORTRAIT`(2:3)、`LANDSCAPE`(3:2) |
| `--size` | 自由比例，优先级高于 `--dimensions` |
| `--speed` | `relax` / `fast` / `turbo` |

---

## Describe（图片反推提示词）

上传一张图片，MJ 返回 4 条候选提示词（同步约 1-3s）：

```bash
aigc-cli mj describe --image-url input.png
```

结果在 `prompt` / `description` 字段中，4 条用 `1️⃣` `2️⃣` `3️⃣` `4️⃣` 编号分隔。

| 参数 | 说明 |
|---|---|
| `--image-url` | 图片 URL/路径（必填，单张） |
| `--speed` | `relax` / `fast` / `turbo` |

---

## Edits（图片编辑）

用 prompt + 参考图重写整张图。适合背景替换、风格迁移：

```bash
aigc-cli mj edits \
  --prompt "replace the background with a modern kitchen" \
  --image-url product.png \
  --version "8.1" \
  --speed fast
```

参数同 [Imagine](#imagine文生图--图生图)，但 `--image-url` 为必填。

---

## Upscale（放大）

从 imagine 的 2x2 网格选一张放大（U1-U4）。从现有图裁剪，**毫秒级返回**：

```bash
# 放大的图 1
aigc-cli mj upscale --task-id task_xxx --index 1

# 用 custom_id 直传按钮（绕过自动匹配）
aigc-cli mj upscale \
  --task-id task_xxx \
  --custom-id "MJ::JOB::upsample::1::abc123"
```

| 参数 | 说明 |
|---|---|
| `--task-id` | 父任务 ID（必填） |
| `--index` | 1-4，对应 U1-U4 |
| `--custom-id` | 按钮 customId，设置后跳过 index 匹配 |
| `--speed` | `relax` / `fast` / `turbo` |

### HD Upscale（高清放大）

常规 upscale 只是裁剪，HD upscale 执行真实放大（2x），返回单张高清图，约 60-120s：

```bash
aigc-cli mj upscale \
  --task-id task_xxx \
  --custom-id "MJ::JOB::upsample_v7_2x_subtle::1::abc"
```

---

## Variation（变体）

从 imagine 网格的一张生成变体：

```bash
# 细微变体（V1-V4）
aigc-cli mj variation --task-id task_xxx --index 3

# 强变体
aigc-cli mj high-variation --task-id task_xxx --index 2

# 弱变体
aigc-cli mj low-variation --task-id task_xxx --index 4
```

| 参数 | 说明 |
|---|---|
| `--task-id` | 父任务 ID（必填） |
| `--index` | 1-4，对应 V1-V4 |
| `--custom-id` | 按钮 customId |
| `--speed` | `relax` / `fast` / `turbo` |

---

## Reroll（重新生成）

用父任务的 prompt 重新生成 4 张图（🔄）。**不需要 index**：

```bash
aigc-cli mj reroll --task-id task_xxx
```

---

## Zoom（扩图）

在 upscale 后的单张图上扩图（outpaint）：

```bash
# 1.5x Outpaint（zoom_ratio < 2）
aigc-cli mj zoom --task-id task_xxx --zoom-ratio 1.5

# 2x CustomZoom（zoom_ratio >= 2 或省略）
aigc-cli mj zoom --task-id task_xxx
```

| 参数 | 说明 |
|---|---|
| `--task-id` | Upscale 后的单图任务 ID（必填） |
| `--zoom-ratio` | < 2 用 Outpaint 1.5x，>= 2 或省略用 CustomZoom 2x |
| `--index` | 父任务中的图序号（单图通常不需要） |
| `--custom-id` | 按钮 customId |

---

## Pan（平移）

在 upscale 后的单张图上向指定方向平移：

```bash
aigc-cli mj pan --task-id task_xxx --direction right
aigc-cli mj pan --task-id task_xxx --direction up
```

| 参数 | 说明 |
|---|---|
| `--task-id` | Upscale 后的单图任务 ID（必填） |
| `--direction` | `left` / `right` / `up` / `down` |
| `--index` | 父任务中的图序号 |
| `--custom-id` | 按钮 customId |

> 仅 v6 / v6.1 / v7 / niji 6 支持。v8/v8.1 已移除 pan 按钮。

---

## Inpaint（局部重绘入口）

进入区域重绘（Vary Region）。提交后任务进入 **MODAL** 状态，需要继续调 `modal`：

```bash
# 第一步：进入 MODAL
aigc-cli mj inpaint --task-id task_xxx

# 返回 {"status": "modal", "task_id": "task_yyy"}
```

| 参数 | 说明 |
|---|---|
| `--task-id` | Upscale 后的单图任务 ID（必填） |
| `--index` | 图中的序号（单图通常不需要） |
| `--custom-id` | 按钮 customId |

---

## Modal（提交 Inpaint 参数）

给 MODAL 状态的任务提交遮罩 + prompt：

```bash
aigc-cli mj modal \
  --task-id task_yyy \
  --prompt "replace the selected area with a red leather sofa" \
  --mask-url ./mask.png
```

| 参数 | 说明 |
|---|---|
| `--task-id` | inpaint 返回的 local task id（必填，须在 MODAL 状态） |
| `--prompt` / `-p` | 重绘提示词（留空则继承父任务） |
| `--mask-url` | 蒙版图 URL/路径（白色=重绘区域，透明=保留） |
| `--speed` | `relax` / `fast` / `turbo` |

> 进入 MODAL 后 30 分钟内必须调用 modal，否则自动取消退款。

---

## Video（图生视频）

MJ 的 image-to-video，固定 FAST 模式，时长 ~5 秒：

```bash
# 简单 i2v
aigc-cli mj video \
  --image-url cat.png \
  --motion high \
  --batch-size 4

# 使用已有 imagine 任务
aigc-cli mj video \
  --task-id task_xxx --index 0 \
  --animate-mode auto

# 首尾帧过渡
aigc-cli mj video \
  --prompt "transition from sunrise to sunset" \
  --image-url sunrise.jpg \
  --end-url sunset.jpg \
  --video-type vid_1.1_i2v_720
```

| 参数 | 说明 |
|---|---|
| `--image-url` | 首帧图片（必填，除非用 `--task-id`） |
| `--task-id` | 复用已有的 SUCCESS imagine |
| `--index` | imagine 4 图中的哪一帧（0-3） |
| `--prompt` / `-p` | 视频 prompt（可留空） |
| `--video-type` | 分辨率：`vid_1.1_i2v_480`（默认）、`vid_1.1_i2v_720` |
| `--animate-mode` | `manual`（默认）/ `auto`（需 `--task-id` + `--index`） |
| `--motion` | `low` / `high`（默认） |
| `--batch-size` | 1（默认）/ 2 / 4，按倍计费 |
| `--end-url` | 尾帧 URL（自动启用首尾帧模式） |

---

## Remix（重塑，v8/v8.1 专用）

v8 面板的 reshape 操作，可改变 prompt：

```bash
# 强重塑（大幅变化）
aigc-cli mj remix-strong --task-id task_xxx --index 1

# 弱重塑（小幅变化，保留主体）
aigc-cli mj remix-subtle --task-id task_xxx --index 1 --prompt "new style"
```

| 参数 | 说明 |
|---|---|
| `--task-id` | v8/v8.1 父任务 ID（必填） |
| `--index` | 1-4（必填） |
| `--prompt` / `-p` | 新 prompt（留空继承父任务） |
| `--speed` | `relax` / `fast` / `turbo` |

---

## Query（查询任务）

```bash
aigc-cli mj query task_xxx
```

返回完整任务信息，包括 `grid_image_url`、`image_urls`、`buttons`、`prompt` 等。任务完成后自动下载图片到 `--output` 目录。

### Follow-up buttons 说明

查询结果的 `buttons` 列表显示了当前任务支持哪些后续操作。每个 button 的 `customId` 可以直接传给 `--custom-id` 参数，跳过自动匹配：

| 按钮 | 对应子命令 | 说明 |
|---|---|---|
| `U1` ~ `U4` | `upscale --index 1~4` | 放大某张图 |
| `V1` ~ `V4` | `variation --index 1~4` | 细微变体 |
| `🔄` | `reroll` | 重新生成 |
| `⬅` `➡` `⬆` `⬇` | `pan --direction` | 平移 |
| `Zoom Out 1.5×` / `2×` | `zoom --zoom-ratio` | 扩图 |
| `Vary (Region)` | `inpaint` → `modal` | 局部重绘 |

按钮信息也能帮你判断任务状态：

- 有 `U1-U4` → 刚完成 imagine，**还没 upscale**，先 upscale 再 zoom/pan
- 只有 `Zoom Out / Vary (Region)` → 已经 upscale 过了，是单张图
- 没有任何按钮 → 任务可能失败了或还在处理中

HD upsample 按钮不会自动展示在 buttons 列表里（它们是特殊的 custom_id）。如果你需要 HD 放大，从文档的 [HD Upscale](#hd-upscale高清放大) 章节参考对应的 custom_id 格式。

---

## 配置默认值

写入 `~/.config/aigc-cli/config.yaml`：

```yaml
defaults:
  midjourney:
    speed: fast           # relax (默认) / fast / turbo
    version: "6.1"        # MJ 版本
    style: raw
    size: "16:9"
    # niji: false
```

---

## API 端点参考

| 端点 | 对应子命令 |
|---|---|
| `POST /v1/midjourney/generations` | `imagine` |
| `POST /v1/midjourney/generations/imagine` | `imagine` |
| `POST /v1/midjourney/generations/blend` | `blend` |
| `POST /v1/midjourney/generations/describe` | `describe` |
| `POST /v1/midjourney/generations/edits` | `edits` |
| `POST /v1/midjourney/generations/upscale` | `upscale` |
| `POST /v1/midjourney/generations/variation` | `variation` |
| `POST /v1/midjourney/generations/high-variation` | `high-variation` |
| `POST /v1/midjourney/generations/low-variation` | `low-variation` |
| `POST /v1/midjourney/generations/reroll` | `reroll` |
| `POST /v1/midjourney/generations/zoom` | `zoom` |
| `POST /v1/midjourney/generations/pan` | `pan` |
| `POST /v1/midjourney/generations/inpaint` | `inpaint` |
| `POST /v1/midjourney/generations/modal` | `modal` |
| `POST /v1/midjourney/generations/video` | `video` |
| `POST /v1/midjourney/generations/remix-strong` | `remix-strong` |
| `POST /v1/midjourney/generations/remix-subtle` | `remix-subtle` |
| `GET /v1/midjourney/{task_id}` | `query` |
