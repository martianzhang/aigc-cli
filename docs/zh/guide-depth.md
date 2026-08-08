# 深度转换（`aigc-cli depth`）

把任意**图片或视频**转换成**灰度深度图**：画面中像素亮度表示相对距离（近亮远暗），去掉纹理、只保留空间结构。这是深度引导 / 控制视频式图生视频（如 Wan VACE、Kling 动作控制、Vidu 参考生视频）的标准输入：把深度图当动作/空间参考 + 上传一张参考照片，AI 就能生成"保留原空间结构、换成新内容"的作品。

深度估计由本地 **Depth Anything V2** ONNX 模型完成（默认 `depth-anything-v2-small`，Apache-2.0），无需 API Key、无需联网。

## 快速上手

```bash
# 首次使用：安装 ONNX Runtime + 默认深度模型
aigc-cli depth init

# 转换单张图片 → 深度 PNG（按扩展名自动识别）
aigc-cli depth -i photo.jpg

# 转换视频 → 深度 MP4（需要 ffmpeg）
aigc-cli depth -i video.mp4

# 常用选项
aigc-cli depth -i photo.jpg --invert          # 近暗远亮（反转方向）
aigc-cli depth -i photo.jpg --color           # Spectral_r 彩色深度图（近暖远冷）
aigc-cli depth -i photo.jpg --skeleton        # 在深度图上绘制人体骨架（COCO17）
aigc-cli depth -i photo.jpg --face            # 检测人脸，绘制检测框 + 眼睛 + 关键点
aigc-cli depth -i video.mp4 --face            # 在深度视频每一帧上标注人脸
aigc-cli depth -i photo.jpg --preview         # 生成后用系统查看器打开结果
aigc-cli depth -i video.mp4 --start-time 00:01:00 --end-time 00:01:30
aigc-cli depth -i video.mp4 --encode-args "-crf 28 -preset slow"
aigc-cli depth -i photo.jpg --dry-run         # 只打印将执行的命令，不实际运行
```

输入类型按文件扩展名自动识别：

| 输入 | 输出 |
|---|---|
| `.png` / `.jpg` / `.jpeg` / `.webp` / `.bmp` / `.gif` / `.avif` / `.heic` / `.jxl` | `<文件名>_depth.png`（单图推理，无需 ffmpeg） |
| `.mp4` / `.mov` / `.mkv` / `.avi` / `.webm` … | `<文件名>_depth.mp4`（H.264、`yuv420p`、faststart） |

输出默认保存在输入文件旁边，或通过 `--output` 指定路径。

## 参数

| 参数 | 说明 | 默认 |
|---|---|---|
| `--input` / `-i` | 输入图片或视频文件 | 必填 |
| `--output` / `-o` | 输出路径（默认 `<文件名>_depth.png` / `.mp4`） | 自动 |
| `--model` | 深度模型（见下方表格；别名：`small`、`base`、`large`） | `depth-anything-v2-small` |
| `--size` | 推理分辨率（短边，14 对齐）。图片默认 `518`；视频默认 `280`（快）——追求更高质量用 `378` / `518` | 图片 `518` / 视频 `280` |
| `--invert` | 反转深度方向（近暗远亮） | off |
| `--color` | 输出 Spectral_r 彩色深度图（近处暖色红/橙，远处冷色蓝/紫），与官方 Depth-Anything-V2 可视化一致 | off |
| `--start-time` | 视频：开始时间（`SS`、`MM:SS`、`HH:MM:SS`） | 视频开头 |
| `--end-time` | 视频：结束时间；单独用 = 转前 N 秒 | 视频结尾 |
| `--keep-audio` | 视频：保留原视频音轨 | off |
| `--encode-args` | 视频：追加到 ffmpeg 编码命令的自定义参数，**追加在默认参数之后**——同名参数会覆盖默认值（后者生效）。例：`"-crf 28"`（更小文件）、`"-preset slow"`（更优压缩） | CRF 23，preset medium |
| `--no-smooth` | 视频：关闭时序平滑（开启时减轻闪烁） | off（平滑开） |
| `--parallel` / `-p` | 视频：并行推理的 worker 数。按机器核数自动调节（默认 min(性能核, 4)）；长视频可调高，省内存可调低 | 自动 |
| `--preview` | 生成后用系统默认查看器打开结果 | off |
| `--skeleton` | 检测人体姿态，在深度输出上绘制 COCO17 骨架（19 条骨骼、17 个关节点，图片或视频逐帧）。需先 `aigc-cli depth init --skeleton` | off |
| `--face` | 检测人脸（纯 Go pigo 引擎），绘制检测框、眼睛和 15 个面部关键点（图片或视频逐帧）。需先 `aigc-cli depth init --face` | off |
| `--dry-run` | 只打印将执行的命令，不实际运行 | off |

## 模型

| 模型 | 参数量 | 大小 | 许可证 | 说明 |
|---|---|---|---|---|
| `depth-anything-v2-small` | 24.8M | 99MB | Apache-2.0 | **默认。** 最快，输出干净——平滑掉细碎纹理、突出主体结构 |
| `depth-anything-v2-base` | 97.5M | 370MB | CC-BY-NC-4.0 | 质量更高，非商用 |
| `depth-anything-v2-large` | 335M | 1.25GB | CC-BY-NC-4.0 | 最高质量，非商用 |

> **商用注意许可证**：`depth-anything-v2-small` 为 Apache-2.0（可商用）。`-base` 和 `-large` 为 CC-BY-NC-4.0（非商用）。

用 `aigc-cli depth init --model <id>` 下载指定模型，或 `aigc-cli depth init --all` 下载全部变体。

## 依赖

- **图片**：只需 `aigc-cli depth init` 安装的 ONNX Runtime + 模型。
- **视频**：额外需要 **ffmpeg**（aigc-cli 不捆绑），需在 PATH 中。各平台安装方式见 [guide-ffmpeg.md](guide-ffmpeg.md)。

## 技巧

- 图片推理很快：CPU 上约 1–3 秒（视模型而定）。
- 视频推理约 **3 帧/秒**（CPU，V2-Small）：10 秒 30fps 视频约需 2 分钟。建议先用 `--end-time` 转换一小段样片，调好 `--invert` / `--no-smooth` 后再转换全片。
- 深度视频默认无音轨（深度视频是动作参考）；需要原声加 `--keep-audio`。
- `--dry-run` 会打印抽帧和编码两条 ffmpeg 命令，方便你查看或手动调整参数。
- 在 chat 或 MCP 中，对应的工具是 `convert_depth`（同样的自动路由）。
