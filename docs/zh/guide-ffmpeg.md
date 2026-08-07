# ffmpeg 常见命令速查

`aigc-cli video` / `audio` 命令负责**生成与下载**，生成后的后处理（抽帧、缩略图、转 GIF、截取、转码、提取音频）直接用系统自带的 **ffmpeg** 即可，无需额外集成。

> 为什么不用内置？ffmpeg 是事实标准，功能全、生态成熟，直接调用命令最简单可靠。aigc-cli 保持单二进制，不捆绑 ffmpeg。

## 检查安装

```bash
ffmpeg -version   # 未安装：brew install ffmpeg / apt install ffmpeg / choco install ffmpeg
```

## 视频后处理（配合 `video` 命令）

`aigc-cli video --prompt "..."` 生成的 mp4 默认存在当前目录，以下命令以其为输入。

### 抽帧（预览内容）

```bash
# 第 3 秒抽一帧
ffmpeg -i video_xxx.mp4 -ss 3 -frames:v 1 frame_3s.png

# 每秒抽一帧（做联系表/逐帧检查）
ffmpeg -i video_xxx.mp4 -vf fps=1 frame_%03d.png
```

### 首帧缩略图

```bash
ffmpeg -i video_xxx.mp4 -frames:v 1 -vf scale=320:-1 thumb.jpg
```

### 转 GIF（社媒分享）

```bash
# 基础转换（质量尚可）
ffmpeg -i video_xxx.mp4 -vf fps=10,scale=480:-1 out.gif

# 高质调色板模式（推荐，体积小、无抖动）
ffmpeg -i video_xxx.mp4 -vf "fps=12,scale=480:-1:flags=lanczos,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse" out.gif
```

### 截取片段

```bash
# 截取 2s–5s（-t 是时长，不是结束时间）
ffmpeg -i video_xxx.mp4 -ss 2 -t 3 -c copy clip.mp4

# 精确截取（重新编码，时间定位更准）
ffmpeg -ss 2 -i video_xxx.mp4 -t 3 -c:v libx264 -crf 18 clip.mp4
```

### 拼接多个片段

```bash
# 1. 先写文件清单
echo "file 'clip1.mp4'"  > list.txt
echo "file 'clip2.mp4'" >> list.txt

# 2. 拼接（同编码时 -c copy 无损）
ffmpeg -f concat -safe 0 -i list.txt -c copy merged.mp4
```

### 提取音频

```bash
ffmpeg -i video_xxx.mp4 -vn -c:a libmp3lame -q:a 2 audio.mp3
ffmpeg -i video_xxx.mp4 -vn -c:a pcm_s16le audio.wav
```

### 转码 / 压缩体积

```bash
# 转 H.264 + AAC（兼容性最好的上传格式）
ffmpeg -i video_xxx.mp4 -c:v libx264 -crf 23 -c:a aac out.mp4

# 降到 720p 压缩体积
ffmpeg -i video_xxx.mp4 -vf scale=-2:720 -c:v libx264 -crf 26 -c:a aac out_720p.mp4

# 移除音频轨
ffmpeg -i video_xxx.mp4 -an out_silent.mp4
```

> `-crf` 控制质量：18 ≈ 无损观感，23 默认，28 明显压缩。值越小体积越大质量越高。

### 倍速 / 倒放

```bash
ffmpeg -i video_xxx.mp4 -filter:v "setpts=0.5*PTS" -filter:a "atempo=2.0" out_2x.mp4
ffmpeg -i video_xxx.mp4 -vf reverse -af areverse out_reversed.mp4
```

## 音频处理（配合 `audio` 命令）

### 本地 ASR 前转 wav 16k 单声道

本地 sherpa-onnx 识别（`aigc-cli audio asr --local`）需要 16k 单声道 wav：

```bash
ffmpeg -i input.mp3 -ar 16000 -ac 1 out_16k.wav
```

### 其他常见操作

```bash
# 裁剪前 30 秒
ffmpeg -i input.mp3 -t 30 clip.mp3

# 音量提升 2 倍（防削波加 limiter）
ffmpeg -i input.mp3 -af "volume=2.0,alimiter=limit=0.95" louder.mp3

# 拼接两段音频
ffmpeg -i a.mp3 -i b.mp3 -filter_complex "[0:a][1:a]concat=n=2:v=0:a=1" merged.mp3

# TTS 生成后转 mp3 方便分享
ffmpeg -i tts.wav -c:a libmp3lame -q:a 2 tts.mp3
```

## 常用参数速查

| 参数 | 作用 | 示例 |
|---|---|---|
| `-i <file>` | 指定输入 | `-i video.mp4` |
| `-ss <t>` | 从第 t 秒开始 | `-ss 3`（支持 `1:30` 格式） |
| `-t <dur>` | 处理时长 | `-t 3` |
| `-frames:v N` | 只输出 N 帧（抽帧用） | `-frames:v 1` |
| `-vf` | 视频滤镜链（逗号分隔） | `-vf scale=480:-1,fps=10` |
| `-af` | 音频滤镜链 | `-af volume=2.0` |
| `-c:v` | 视频编码器 | `-c:v libx264` / `-c copy` 复制不重编码 |
| `-c:a` | 音频编码器 | `-c:a aac` / `-c:a pcm_s16le` |
| `-crf` | 质量 0-51（越小越好） | `-crf 23` |
| `-an` / `-vn` | 去掉音频 / 视频轨 | `-an` |
| `-y` | 覆盖输出文件 | `-y` |

## 提示

- **`-c copy` 无损且秒级完成**，但要求输入输出同编码；需要重新编码时（裁剪精度、转码）才去掉它。
- **滤镜顺序敏感**：`fps` 应在 `scale` 之后，先降帧再缩放更省 CPU。
- 更多用法：`ffmpeg -h`、官方文档 <https://ffmpeg.org/documentation.html>。
