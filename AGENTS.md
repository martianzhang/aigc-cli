# AGENTS.md — AI 助手工作指南

> 本文档面向 AI 编码助手（Claude / OpenCode / Cursor 等），定义在本项目中工作时的约束和流程。

---

## 一、构建系统

本项目的所有构建、格式化、测试、覆盖率均通过 **Makefile** 管理，**禁止直接调用 `go build`、`go test` 等命令**。

| 命令 | 作用 | 必须运行 |
|---|---|---|
| `make fmt` | `go fmt ./...` 格式化代码 | 每次编辑后 |
| `make build` | 编译二进制 | 每次编辑后 |
| `make lint` | `go vet ./...` + `golangci-lint` 静态检查 | 每次提交前 |
| `make test` | 运行全部测试 | 每次编辑后 |
| `make cover` | 测试覆盖率报告 | 每次提交前 |
| `make clean` | 清理构建产物 | 按需 |
| `make run ARGS="..."` | 编译并运行 | 手动验证时 |
| `make release` | 跨平台交叉编译 | CI 自动执行 |

> **永远不要直接调用 `go build`、`go test`、`go fmt`**。统一走 Makefile。

---

## 二、开发工作流（强制遵守）

每次修改代码后，**必须**按以下顺序执行：

```
代码修改 → make fmt → make build → make test → 同步文档
```

### 2.1 修改代码

- 遵循 Go 标准风格，提交前必须 `make fmt`
- 内部包导入顺序：标准库 → 第三方 → 内部包，组间空行分隔

### 2.2 编译验证

```bash
make fmt && make build
```

编译必须通过。编译产物在项目根目录（`aigc-cli.exe` 或 `aigc-cli`）。

### 2.3 运行测试

```bash
make test
```

所有测试必须通过。若测试失败：

1. 确认是否为**已有失败**（`git stash` 后跑一遍对比）
2. 如果是自己引入的：修复代码
3. 如果是已有失败：在变更中注明

### 2.4 覆盖率检查

```bash
make cover
```

重点关注变更文件的覆盖率变化趋势，非硬性门槛。

### 2.5 调试原则

调试时**优先使用 CLI 现有的诊断开关**（`-v/--verbose` 详细输出、`--dry-run` 打印等价 curl、既有日志），**而不是在代码里临时插入 `fmt.Println`/调试打印**。

理由：临时调试代码调试结束后必须手动清理，容易残留进提交；而诊断开关是产品能力，无需清理，也不会污染代码。

若诊断开关仍不足以定位问题，再考虑临时埋点——但提交前务必删除，或用 `// TODO(debug)` 明确标记以便复查。

---

## 三、文档同步规则（⚡ 硬性要求）

**文档不得滞后于代码。** 任何功能性变更（新增/修改命令、参数、行为）都必须同步更新文档。

### 3.1 需要同步的文档

| 文档 | 触发条件 |
|---|---|
| `README.md` | 新增/删除命令、修改用法、新增依赖 |
| `docs/installation.md` | 修改安装方式、环境变量、配置路径 |
| `docs/guide-image.md` | 修改 `image` 命令参数或行为 |
| `docs/guide-video.md` | 修改 `video` 命令参数或行为 |
| `docs/guide-chat.md` | 修改 `chat` 命令参数或行为 |
| `docs/guide-detect.md` | 修改 `detect` 命令参数或行为 |
| `docs/guide-watermark.md` | 修改水印引擎（新增/修改平台、算法变更、alpha map） |
| `docs/guide-midjourney.md` | 修改 `midjourney` 命令参数或行为 |
| `docs/guide-ideas.md` | 修改 `ideas` 命令参数或行为 |
| `docs/guide-commands.md` | 修改 `models`/`task`/`balance`/`dry-run` 等辅助命令 |
| `docs/faq.md` | 新增常见问题 |
| `docs/mcp.md` | 修改 MCP 工具定义或配置方式 |
| `docs/config.example.yaml` | 新增/修改配置字段 |
| `docs/release_notes/vX.Y.Z.md` | 每次发版前创建 |

### 3.2 注意事项

- 文档要写**用户视角**，而非实现细节
- 示例命令必须真实可运行
- 新增 flag 要同时在 `--help` 和对应 guide 文档中体现
- **例外**：纯重构、修 typo、内部测试代码变更无需更新文档，但要在 commit message 中注明 `(no-doc)`

---

## 四、代码规范

### 4.1 导入顺序

```go
import (
    "fmt"           // 标准库
    "os"

    "github.com/spf13/cobra"  // 第三方

    "github.com/martianzhang/apimart-cli/internal/types"  // 内部包
)
```

### 4.2 错误处理

- 错误包装用 `%w`，不是 `%v`
- 错误消息首字母小写
- CLI 层返回 error，由 `cmd.Execute()` 统一处理

### 4.3 SilenceUsage

所有有 `RunE` 的 cobra 命令**必须**设置 `SilenceUsage: true`。因为 `RunE` 返回的错误通常是运行时错误（API 调用失败、网络超时等），不是参数解析错误。显示 Usage 会干扰用户查看真正的错误信息。

```go
// ✅ 正确
var chatCmd = &cobra.Command{
    Use:          "chat",
    SilenceUsage: true,
    RunE: runChat,
}

// ❌ 错误 — 运行时错误也会打印 Usage
var chatCmd = &cobra.Command{
    Use:   "chat",
    RunE: runChat,
}
```

### 4.4 变量命名

- Go 驼峰式：`APIKey`、`HTTPProxy`、`baseURL`
- 不要用拼音命名，不要用单字母变量（循环变量除外）

### 4.5 配置优先级

```
CLI 参数 > JSON 输入 > YAML 配置 > 代码默认值
```

修改配置相关代码时，务必维护此优先级。

### 4.6 提交信息

```
<type>(<scope>): <简短描述>

<可选详细描述>
```

type: `feat` / `fix` / `refactor` / `docs` / `test` / `chore` / `style`
scope: `image` / `video` / `chat` / `ideas` / `midjourney` / `mcp` / `config` / `docs` / `skill`

---

## 五、项目架构

```
aigc-cli/
├── cmd/              # cobra 命令定义（薄层：解析参数→调用逻辑→输出结果）
│   ├── image.go      # 图片生成
│   ├── video.go      # 视频生成
│   ├── chat.go       # AI 对话
│   ├── ideas.go      # 提示词灵感搜索
│   └── ...
├── internal/
│   ├── client/       # HTTP API 客户端（APIMart / OpenAI / OpenRouter / 云雾）
│   ├── config/       # Viper 配置加载（YAML + 环境变量）
│   ├── mcp/          # MCP Server 实现
│   ├── provider/     # Provider 检测（APIMart / OpenAI / OpenRouter）
│   └── types/        # 请求/响应数据结构和配置类型
├── docs/             # 用户文档
│   ├── release_notes/ # 各版本 release notes
├── skills/           # AI Agent SKILL 定义
├── main.go           # 入口
├── Makefile          # 统一构建入口
├── AGENTS.md         # ← 当前文件
```

### 5.1 HTTP 代理（http_proxy）

配置了 `http_proxy` 后，**所有** HTTP 请求都必须走代理，包括：
- API 调用（image/video/chat/balance/task 等） — 通过 `client.New()` 创建的客户端
- 文件下载（下载生成的图片/视频） — 使用 `http.Get()` / `http.DefaultClient`
- 非 API 请求（如 ideas 搜索、模型定价查询）

实现方式：在启动入口（`root.go` / `mcp.go` 的 `PersistentPreRunE`）调用 `client.ConfigureDefaultClient(proxyURL)` 配置全局 `http.DefaultClient`。所有使用 `http.Get()`、`http.DefaultClient` 或自定义 HTTP 客户端的地方**不要自行构建 transport**，应复用 `http.DefaultClient` 以自动继承代理配置。

> 新增 HTTP 调用时，优先使用 `http.DefaultClient`，不要新建 `http.Client` 或使用裸 `http.Get()` 以外的 `Transport` 配置。

### 5.2 关键设计决策

- **Provider 检测**集中到 `internal/provider`，新增 provider 只需改此包和策略表
- **策略路由**（`imageStrategies` / `videoStrategies`）用 match-run 模式派发到不同后端
- **文件上传**在 client 层自动处理本地路径→URL 转换
- **配置文件**位于 `~/.config/aigc-cli/config.yaml`

### 5.2 已知技术债务

---

## 六、测试策略

由于大部分 API 接口（图片生成、视频生成、对话）是**付费接口**，无法在 CI 中无成本调用，测试策略如下：

### 6.1 可以无成本测试的（必须覆盖）

- 配置加载与合并（`internal/config` — 已有 91.7%）
- Provider 检测（`internal/provider` — 已有 93.3%）
- 类型序列化/反序列化（`internal/types` — 已有 76.5%）
- CLI 参数解析与校验（`cmd/` — 当前仅 18%，需加强）
- HTTP 请求构建与 curl 生成（`cmd/` 中的 buildXxxCurl）
- 无外部依赖的纯函数（文件名提取、URL 解析等）

### 6.2 需要 mock 的（逐步推进）

- Client 层的请求/响应处理（`internal/client` — 当前 16.3%）
- MCP handler 逻辑（`internal/mcp` — 当前 9.2%）
- 命令的完整执行路径

### 6.3 新增代码原则

- 纯函数必须写表驱动测试
- 重构时优先提取可测试的纯函数
- mock 测试优先于集成测试

---

## 六-B、scripts/ 辅助脚本说明

新增或修改去水印功能时，必须用以下脚本验证效果。所有脚本依赖 `pip install Pillow numpy`。

### 6B.1 check_watermark.py — 可视化诊断

并排对比原图和 clean 图，生成差异热力图，标注已知水印的预期位置。

```bash
# 自动查找 *_clean.*
python scripts/check_watermark.py <原图>

# 指定 clean 图 + 详细报告
python scripts/check_watermark.py <原图> <clean图> --report

# 只看水印位置标柱（无 clean 图时）
python scripts/check_watermark.py <原图> --no-clean

# 示例
python scripts/check_watermark.py .testdata/doubao-snap/PixPin_xxx.png --report
```

输出文件：
- `<原图名>_diff_compare.png`  并排对比（原图 | clean | 热力图）
- `<原图名>_diff_heatmap.png`  单独的热力图
- `<原图名>_wm_region.png`     水印区域裁剪放大图

支持的水印类型（`WATERMARK_CONFIGS` 字典，在脚本顶部）：
- `doubao-snap`  — 豆包网页截图 "AI 生成" 左上角 118x58
- `baidu`        — 百度 "百度 AI生成" 右下角 139x42
- `doubao`       — 豆包嵌入水印 右下角（动态尺寸）
- `zhipu`        — 智谱清言 "智谱清言" 右下角 234x60（@1024px 短边缩放）

**新增水印类型时**，必须在 `WATERMARK_CONFIGS` 中添加对应条目（大小、位置计算函数），保持与 Go 代码 `badge.go` 中 `Register(Config{...})` 配置一致。

判断标准（`--report` 输出）：
- 平均差异 < 2/255  → 移除失败（检测可能未触发）
- 平均差异 2~10    → 移除不完整
- 平均差异 10~30   → 部分处理
- 平均差异 > 30    → 处理成功

### 6B.2 verify_watermark.py — 量化验证

输出 PASS/WARN/FAIL 判断，适合 CI 回归测试。支持 `doubao`、`jimeng`、`gemini`、`baidu`、`zhipu` 等嵌入水印。

```bash
# 自动检测水印位置
python scripts/verify_watermark.py <原图> <clean图>

# 指定 producer
python scripts/verify_watermark.py <原图> <clean图> --producer doubao

# 手动指定水印位置（非标准图片）
python scripts/verify_watermark.py <原图> <clean图> --wm-x 1686 --wm-y 1931 --wm-w 335 --wm-h 83

# 裁剪水印区域保存
python scripts/verify_watermark.py <原图> <clean图> --producer doubao --crop
```

退出码：0=完全移除，1=部分残留，2=错误。

### 6B.3 generate_alpha_go.py — 水印 alpha map 生成

将水印 alpha map PNG 图片转换为 Go 源代码中的 float64 数组，用于新增水印类型时注册。

```bash
# 基本用法
python scripts/generate_alpha_go.py alpha.png modelAlphaRaw

# 指定包名和输出路径
python scripts/generate_alpha_go.py alpha.png modelAlphaRaw \
    --pkg watermark --output internal/watermark/model_alpha.go

# 带注释说明
python scripts/generate_alpha_go.py alpha.png myAlphaRaw \
    --comment "MyModel visible watermark, 200x50"

# 修剪透明边缘（用于抠出浮动水印的实际区域）
python scripts/generate_alpha_go.py alpha.png modelAlphaRaw \
    --trim --trim-threshold 0.05 --pad 2

# 清理低 alpha 背景噪声（用于 UI badge 水印，背景有 ~0.10 半透明值）
python scripts/generate_alpha_go.py alpha.png badgeAlphaRaw \
    --floor 0.10
```

输入格式：
- 灰度图：像素值/255 = alpha
- RGB/RGBA：max(R,G,B)/255 = alpha

### 6B.4 visible_alpha_solve.py — 两拍法提取精确 alpha map

从**黑底+灰底两张受控截图**中数学求解嵌入水印的精确 alpha map。
这是参考项目（wiltodelta/remove-ai-watermarks）使用的标准方法，
对 alpha-blended 嵌入水印的提取精度达到 NCC 0.9998。

```bash
# 基本用法（黑底+灰底两张图）
python scripts/visible_alpha_solve.py <厂商名> <黑底图> <灰底图>

# 示例
python scripts/visible_alpha_solve.py kling kling_black.png kling_gray.png

# 左上角水印（默认右下角，用 --corner bl 切换）
python scripts/visible_alpha_solve.py samsung samsung_black.png samsung_gray.png --corner bl
```

输出文件（到 `scripts/assets/`）：
- `<name>_alpha.png`          精确 alpha map（可直接用于 `generate_alpha_go.py`）
- `<name>_alpha_data.go`      含 `Register(Config{...})` 的 Go 源文件（需核对后使用）

同时打印几何参数常量，用于填写 `PositionResolver`。

---

## 新增水印通用流程

嵌入水印和 UI badge 是两种不同性质的水印，新增支持的操作流程不同。

### 流程 A：嵌入水印（两拍法，推荐）

适用于 AI 平台生成图片时自带的 alpha-blended 水印（如豆包、即梦）。

**Step 1: 准备纯色种子图**

用任意绘图工具生成三张 **2048x2048** 的纯色 PNG：

| 文件名 | 颜色 | 用途 |
|---|---|---|
| `seed_black.png` | RGB(0, 0, 0) | 定位水印位置 + 直接反推 alpha |
| `seed_gray.png` | RGB(128, 128, 128) | 精确求解 alpha（消除渐变背景干扰） |
| `seed_white.png` | RGB(255, 255, 255) | 验证水印颜色为白色（备用） |

纯黑和纯灰是必须的，纯白可选（仅用于交叉验证）。

**Step 2: 到 AI 平台生成带水印的图**

打开目标 AI 平台，用**文生图**功能（不是图生图），使用以下 prompt：

生成纯黑底图：
```
帮我画 请生成一张纯黑色图片，RGB(0,0,0)，不要添加任何内容。比例 1:1
```

生成纯灰底图（关键：要写具体数字，AI 容易把"灰色"理解为浅灰）：
```
帮我画 请生成一张中灰色图片，RGB(128,128,128)，不要添加任何内容。比例 1:1
```

生成纯白底图（可选，用于交叉验证）：
```
帮我画 请生成一张纯白色图片，RGB(255,255,255)，不要添加任何内容。比例 1:1
```

英文版 prompt（部分平台不支持中文）：

```
Generate a pure black image, RGB(0,0,0), no content. Aspect ratio 1:1.
Generate a pure gray image, RGB(128,128,128), no content. Aspect ratio 1:1.
Generate a pure white image, RGB(255,255,255), no content. Aspect ratio 1:1.
```

> ⚠️ 关键原则：
> - **必须下载原始输出文件**（原始 PNG/JPEG），**不能截图**
> - 不要裁剪、编辑或重新保存
> - 确保平台的"添加水印"选项是开启状态
> - 如果平台支持多种分辨率，每种分辨率都生成一份，**确保所有图片分辨率一致**
> - 下载后检查灰底图的 RGB 值是否接近 128（用 PS/画图打开看），如果 AI 生成的灰色不对，重试或调整提示词

**Step 3: 命名规则**

```
<厂商名>_black_<宽>x<高>_<序号>.png      # 黑底
<厂商名>_gray_<宽>x<高>_<序号>.png       # 灰底
<厂商名>_white_<宽>x<高>_<序号>.png      # 白底（可选）
```

例如：
```
baidu_black_2048x2048_1.png
baidu_gray_2048x2048_1.png
```

**Step 4: 运行两拍法脚本**

```bash
# 提取 alpha map + 生成 Go 注册代码
python scripts/visible_alpha_solve.py baidu baidu_black.png baidu_gray.png

# 输出：
#   scripts/assets/baidu_alpha.png       ← 精确 alpha map
#   scripts/assets/baidu_alpha_data.go   ← Go 注册代码（需核对）
```

**Step 5: 核对并整合到项目中**

1. 确认 `baidu_alpha_data.go` 中的 `Type`、`Name`、`PositionResolver` 正确
2. 在 `internal/watermark/types.go` 中添加对应的 `Type` 常量
3. 在 `internal/watermark/` 下创建对应的引擎文件（参考 `doubao.go`、`jimeng.go`）
4. 在 `scripts/check_watermark.py` 的 `WATERMARK_CONFIGS` 中添加对应条目（用于可视化诊断）
5. `make fmt && make build && make test`
6. 用 `check_watermark.py --report` 验证效果

**备用方案**：如果平台没有"图生图"功能，无法生成纯色底图，可生成 10-12 张**内容不同但分辨率相同**的普通图片，水印是唯一共同元素，通过逐像素取 min/median 来分离水印。

### 流程 B：UI Badge 水印（仅检测）

适用于浏览器截图中的 CSS 渲染 badge（如 `doubao-snap`）。

这类水印不是 alpha-blended，两拍法不适用，只能用截图方式建立 alpha map。
alpha map 精度有限，只用于 AIGC 检测（forensic 信号），不走去除流程。

```bash
# Step 1: 截图 badge 区域为 PNG
# Step 2: 用 generate_alpha_go.py 生成 Go 源文件
python scripts/generate_alpha_go.py badge.png badgeNameAlphaRaw \
    --pkg watermark \
    --output internal/watermark/badge_name_alpha_data.go \
    --comment "Xxx watermark badge, 200x50" \
    --floor 0.10

# Step 3: 在 types.go 中添加 TypeXxxBadge 常量
# Step 4: 在 badge.go 中用 Register 注册，RemoveStrategy 设为 RemoveSkip
# Step 5: make fmt && make build && make test
```

---

## 七、Release 流程

### 7.1 每次发版前

打 tag 前**必须**在 `docs/release_notes/` 下创建对应版本的 release notes 文件：

```
docs/release_notes/vX.Y.Z.md
```

CI 脚本（`.github/workflows/ci.yml`）在 push tag 时会自动读取该文件作为 GitHub Release 的 notes 内容。如果文件不存在，会自动 fallback 到 `--generate-notes`。

### 7.2 标准发版步骤

```bash
# 1. 编写 release notes
echo "..." > docs/release_notes/v1.2.3.md

# 2. 提交 release notes
git add docs/release_notes/v1.2.3.md
git commit -m "docs: add release notes for v1.2.3"

# 3. 打 tag
git tag v1.2.3

# 4. 推送（CI 自动构建并发布）
git push origin main --tags
```

### 7.3 版本号规则（语义化版本）

遵循 [SemVer](https://semver.org/) 规范：

| 版本 | 场景 | 示例 |
|---|---|---|
| **v0.Y.Z** (minor) | 新增功能、新命令、新配置项 | v0.5.0 → v0.6.0 |
| **v0.Y.Z** (patch) | Bug 修复、重构、文档、测试 | v0.5.0 → v0.5.1 |

判断标准：
- **有新增功能**（新命令、新参数、新配置项、新工具）→ bump minor (`v0.5.0` → `v0.6.0`)
- **只有修 bug、重构、文档、测试** → bump patch (`v0.5.0` → `v0.5.1`)
- **破坏性变更**（删除命令、修改 flag 名、不兼容的配置变更）→ bump major（v0 阶段暂不适用）

### 7.4 已发布版本的 release notes 补录

如果某个版本发布时没有 notes 文件（v0.5.0 之前），补写文件后可以用 `gh` 命令同步到 GitHub：

```bash
# 写好文件后，更新已有 release
gh release edit v0.5.0 --notes-file docs/release_notes/v0.5.0.md
```

---

## 八、禁止行为

| 禁止事项 | 说明 |
|---|---|
| ❌ 直接调用 `go build` / `go test` / `go fmt` | 必须走 Makefile |
| ❌ 修改代码后不跑 `make build` | 必须确保编译通过 |
| ❌ 修改代码后不跑 `make test` | 必须确保测试通过 |
| ❌ 功能变更不同步文档 | 文档不得滞后代码 |
| ❌ 使用 `as any` / `@ts-ignore` | Go 没有，但任何时候不要抑制类型检查 |
| ❌ 多余的空 `catch` 块 | Go 中没有 try-catch，但不要吞错误 |
| ❌ 主动 commit | 除非用户明确要求 "commit"，否则不得提交代码。讨论阶段的修改先暂存，确认后再统一提交 |
| ❌ 提交前不检查变更文件的 LSP 诊断 | 确保新增代码无警告 |
