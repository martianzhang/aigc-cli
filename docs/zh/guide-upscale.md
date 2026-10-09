# 图像放大（`aigc-cli upscale`）

用本地 AI 模型把图片放大 **2x / 4x**，补足细节而不是简单插值拉伸。基于 **Real-ESRGAN / Real-CUGAN / Swin2SR** 的 ONNX 权重，复用 `background` / `depth` / `ocr` 等命令同一套 ONNX Runtime，**完全离线、无需 API Key**。

大图采用**分块（tile）推理 + 重叠羽化**，内存占用与图片大小无关；块之间无可见接缝。

## 快速上手

```bash
# 首次使用：安装 ONNX Runtime + 默认模型
aigc-cli upscale init

# 放大一张图片（默认通用 4x 模型 → <文件名>_upscaled.png）
aigc-cli upscale -i photo.jpg

# 二次元 / 插画：用 Real-CUGAN 2x
aigc-cli upscale -i art.png --model real-cugan-2x

# 指定输出文件
aigc-cli upscale -i photo.jpg -o photo_4x.png

# 真实世界/压缩照片：Swin2SR RealWorld 4x
aigc-cli upscale -i noisy.jpg --model swin2sr-realworld-x4

# 查看模型清单与许可证 / 已安装模型
aigc-cli upscale init --list
aigc-cli upscale init --list-installed

# 只打印将执行的操作
aigc-cli upscale -i photo.jpg --dry-run
```

输出默认是 PNG，路径为 `<输出目录>/<文件名>_upscaled.png`（输出目录由全局 `--output` / `-o` 控制，默认当前目录）。

## 参数

| 参数 | 说明 | 默认 |
|---|---|---|
| `--input` / `-i` | 输入图片文件 | 必填 |
| `--output` / `-o` | 输出文件路径 | `<文件名>_upscaled.png` |
| `--model` | 模型 id（见下表） | `realesr-general-x4v3` |
| `--scale` | 输出倍率。**省略则用模型原生倍率**；设为其它值会用 CatmullRom 重采样到该倍率 | 模型原生倍率 |
| `--dry-run` | 只打印计划，不实际推理 | off |
| `--preview` | 生成后用系统默认查看器打开结果 | off |

`upscale init` 参数：

| 参数 | 说明 |
|---|---|
| `--model` | 要下载的模型 id（可重复） |
| `--list` | 列出全部模型及许可证 |
| `--list-installed` | 列出已安装模型 |
| `--force` | 即使已存在也重新下载 |

## 模型

| 模型 | 倍率 | 大小 | 许可证 | 适用场景 |
|---|---|---|---|---|
| `realesr-general-x4v3` | 4x | 4.9MB | BSD-3-Clause | **默认。** 通用 4x：速度与质量均衡，日常首选 |
| `real-esrgan-x4plus` | 4x | 67MB | BSD-3-Clause | 高清通用 4x：真实照片/纹理效果最佳，较慢 |
| `real-esrgan-x4plus-anime-6b` | 4x | 18MB | BSD-3-Clause | 二次元/插画 4x（6-block）：边缘干净、色块平整，适合线稿 |
| `real-esrgan-x4plus-anime-4b32f` | 4x | 5.2MB | BSD-3-Clause | 极小 4-block 二次元 4x：比 6B 更小更快，细节略弱 |
| `real-esrgan-animevideov3` | 4x | 2.5MB | BSD-3-Clause | 最轻量二次元/视频 4x：最快，适合动画帧/干净线稿，细节较弱 |
| `real-cugan-2x` | 2x | 5.2MB | MIT | 二次元 2x：线条保留与去噪强，适合插画 |
| `swin2sr-lightweight-x2` | 2x | 8.1MB | Apache-2.0 | 轻量通用 2x：快、资源占用小 |
| `swin2sr-realworld-x4` | 4x | 53MB | Apache-2.0 | 真实/压缩降质 4x：适合有噪点或 JPEG 压缩痕迹的照片 |
| `swin2sr-classical-x4` | 4x | 55MB | Apache-2.0 | 经典降质 4x：干净图像上保真度高 |
| `swin2sr-compressed-x4` | 4x | 55MB | Apache-2.0 | 重度 JPEG 压缩 4x：恢复细节并抑制块状伪影 |

> **许可证**：模型权重保留各自上游许可证——Real-ESRGAN（BSD-3-Clause）、Real-CUGAN（MIT）、Swin2SR（Apache-2.0）。`aigc-cli upscale init --list` 会显示每个模型的许可证。

### 如何选择模型

| 你的素材 | 建议模型 |
|---|---|
| 普通照片（默认、快） | `realesr-general-x4v3` |
| 照片追求更高清晰度（更慢） | `real-esrgan-x4plus` |
| 二次元 / 插画 / 线稿 | `real-cugan-2x`（2x）或 `real-esrgan-x4plus-anime-6b` |
| 二次元（比 6B 更小更快） | `real-esrgan-x4plus-anime-4b32f` |
| 动画帧 / 视频帧（求快） | `real-esrgan-animevideov3` |
| 噪点多 / 有 JPEG 压缩痕迹的实拍 | `swin2sr-realworld-x4` |
| 重度 JPEG 压缩（块状伪影重） | `swin2sr-compressed-x4` |
| 干净图像、要 4x 经典保真 | `swin2sr-classical-x4` |
| 只要 2x、轻量通用 | `swin2sr-lightweight-x2` |

```bash
aigc-cli upscale init --model real-cugan-2x --model swin2sr-lightweight-x2
```

## 工作原理

- 输入按 **256×256 瓦片**、**16px 重叠**切分，逐块推理后按线性羽化权重融合回整图——避免大图一次性进模型导致的内存爆炸与块间接缝。
- 图像尺寸任意（无需是 64 的倍数）；瓦片本身满足各模型的窗口对齐要求。
- 输出上限：单边 **16384px**、总量 **2 亿像素**。超限会报错并提示改用更小 `--scale`。
- 模型均不输出 alpha 通道，**透明通道会被丢弃**（输出为不透明 PNG）。

## 模型存放位置

模型下载到 `<models_dir>/upscale/`（默认 `~/.config/aigc-cli/models/upscale/`）。ONNX Runtime 与其它本地命令**共用**（`<models_dir>/libonnxruntime.*`），只下载一次。`init` 会遵循配置里的 `http_proxy`。

## 配置

`upscale` 是**纯本地**命令，通过命名 provider 指定（`type: local`），与 `ocr` 一致——`models_dir` 由 provider 提供，不在 `upscale` 段里单独配置：

```yaml
defaults:
  upscale:
    provider: "my-local"          # providers 里 type: local 的条目（不填则用默认模型目录）
    model: "realesr-general-x4v3" # 默认模型；--model 可覆盖
providers:
  my-local:
    type: local
    models_dir: "/data/aigc-models"   # 可选；默认 ~/.config/aigc-cli/models
```

- 模型优先级：`--model` > `defaults.upscale.model` > 内置默认（`realesr-general-x4v3`）。
- 模型目录：`providers.<name>.models_dir` > 默认目录（`~/.config/aigc-cli/models`）。

## 技巧

- 通用照片优先 `realesr-general-x4v3`（快）或 `real-esrgan-x4plus`（更清晰，慢）；二次元优先 `real-cugan-2x`。
- 2x 模型（Real-CUGAN / Swin2SR-light）配合 `--scale 4` 会先 2x 再重采样到 4x——想要真正的 4x 细节请用原生 4x 模型。
- 速度参考（Apple Silicon CPU）：512×512 输入用 `realesr-general-x4v3` 约几秒；越大越慢，与像素数成正比。
- 视频超分**暂未内置**：可先抽帧 → 逐帧 `upscale` → 重新编码（参考 `depth` 的视频流程），但逐帧独立推理会有时序闪烁。
