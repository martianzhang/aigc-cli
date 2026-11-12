# SPEC.md — 代码规范

> 被 @AGENTS.md 引用；写代码前需先读取本文件（V2 不再自动加载外部指令文件）。本文档定义写代码时必须遵守的 Go 编码规范。

## 1. 导入顺序

标准库 → 第三方 → 内部包，组间空行分隔：

```go
import (
	"fmt"           // 标准库
	"os"

	"github.com/spf13/cobra" // 第三方

	"github.com/martianzhang/aigc-cli/internal/types" // 内部包
)
```

## 2. 错误处理

- 错误包装用 `%w`，不是 `%v`
- 错误消息首字母小写；专有名词缩写（`API`、`TTS`、`RMBG`、`URL`、`HTTP` 等）可保留大写
- CLI 层返回 error，由 `cmd.Execute()` 统一处理
- 不写空 `catch` 块 / 不吞错误

## 3. SilenceUsage

所有有 `RunE` 的 cobra 命令**必须**设置 `SilenceUsage: true`。因为 `RunE` 返回的错误通常是运行时错误（API 调用失败、网络超时等），不是参数解析错误。显示 Usage 会干扰用户查看真正的错误信息。

根命令（`cmd/root.go`）已设置 `SilenceUsage` / `SilenceErrors`，cobra 会将其继承到子命令；新子命令仍建议显式设置，避免脱离根命令单独使用时行为变化。

```go
// ✅ 正确
var chatCmd = &cobra.Command{
	Use:          "chat",
	SilenceUsage: true,
	RunE:         runChat,
}

// ❌ 错误 — 运行时错误也会打印 Usage
var chatCmd = &cobra.Command{
	Use:   "chat",
	RunE:  runChat,
}
```

## 4. 变量命名

- Go 驼峰式：`APIKey`、`HTTPProxy`、`baseURL`
- 不要用拼音命名，不要用单字母变量（循环变量除外）

## 5. 配置访问器（⚡ 防长链 nil 判断）

**禁止**手写 `shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Chat != nil && ...` 这类逐级 nil 判断。

一律通过访问器 / 泛型工具获取（实现在 `internal/options/config_defaults.go`）：

```go
cfg := options.ChatDefaults()      // *types.ChatDefaults，nil-safe
cfg := options.ImageDefaults()     // *types.ImageDefaults
cfg := options.VideoDefaults()     // *types.VideoDefaults
cfg := options.AudioDefaults()     // *types.AudioDefaults
cfg := options.KnowledgeDefaults() // *types.KBDefaults
cfg := options.DetectConfig()      // *types.DetectConfig
```

访问器返回 nil 时再判空：

```go
if cfg := options.ChatDefaults(); cfg != nil && cfg.Model != "" {
	options.Shared.Model = cfg.Model
}
```

新增配置 section 时，在 `internal/options/config_defaults.go` 用 `field` 组合添加访问器：

```go
func MyDefaults() *types.MyDefaults {
	return field(DefaultsOrNil(), func(d *types.ConfigDefaults) *types.MyDefaults { return d.My })
}
```

通用工具（`internal/options/ptr.go`）：
- `field(p, f)`：单步安全导航，nil 短路，可链式组合
- `deref(p, def)`：指针安全取值，nil 回退默认值

## 6. 配置优先级

```
CLI 参数 > JSON 输入 > YAML 配置 > 代码默认值
```

修改配置相关代码时，务必维护此优先级。

## 7. 提交信息

```
<type>(<scope>): <简短描述>
```

type: `feat` / `fix` / `refactor` / `docs` / `test` / `chore` / `style`
scope: `image` / `video` / `chat` / `ideas` / `midjourney` / `mcp` / `config` / `docs` / `skill`

## 8. 文件规模与共享工具

### 8.1 文件规模与分层

单个 `.go` 文件尽量控制在 200 行以内（不含空白行和注释）。超过后建议按功能域拆分到独立文件，便于 AI 分析。

当前结构：

- `cmd/`：仅 cobra wiring。每个命令文件通常只做 `rootCmd.AddCommand(<pkg>.Cmd())`。
- `internal/cli/<cmd>/`：命令定义、flag 解析、输出格式；业务编排尽量下沉。
- `internal/service/`、`internal/client/`、`internal/provider/`、`internal/knowledge/` 等：可复用业务逻辑，**不得 import `internal/cli`**（depguard 已强制）。

示例：`internal/cli/image/`（原 `cmd/image.go` 拆分后）

| 文件 | 职责 |
|---|---|
| `image.go` | 命令定义、`runImageGenerate`、标志注册、`init` |
| `image_request.go` | 请求构建（`buildImageRequest`、`buildImageCurl`）和输入解析 |
| `image_dispatch.go` | 策略上下文/类型/路由表 |
| `image_runners.go` | 各 Provider 执行函数和 `downloadImages` |
| `image_helpers.go` | Image 模块专有辅助函数 |

`internal/cli/video/` 同理：`video.go` / `video_request.go` / `video_dispatch.go` / `video_runners*.go` / `video_helpers.go`。

### 8.2 共享工具

跨命令复用的功能放在 `internal/` 下的独立包，不挂靠在某个命令目录里。当前主要共享包：

| 包 | 职责 |
|---|---|
| `internal/service` | 下载、输入解析、图片编解码入口、输出掩码等 |
| `internal/fsutil` | 原子写、私有文件权限、临时文件清理 |
| `internal/httpx` | GET/HEAD 重试、`Retry-After`、响应体大小上限 |
| `internal/modelstore` | 模型下载、Range 续传、SHA256 校验 |
| `internal/argutil` | ffmpeg 风格参数拆分 |
| `internal/wait` | context-aware 休眠（轮询/限速） |
| `internal/imgcodec` | 图片编解码 + 解码炸弹上限 |
| `internal/terminal` | 标题/进度条、表格渲染 |
| `internal/options` | 共享配置状态、访问器、client factory、flag 工具 |

**规则**：新增的跨命令公共函数必须放在对应 `internal/*` 包中，不要复制到某个命令目录。若一个函数不依赖具体命令的业务逻辑，它就是共享工具函数。

### 8.3 context 与 I/O

- 所有 HTTP 请求使用 `http.NewRequestWithContext`；CLI 客户端通过 `options.NewClient` 自动绑定进程 context。
- 轮询/限速用 `wait.Sleep(ctx, d)`，禁止裸 `time.Sleep`（会忽略 Ctrl-C）。
- 库代码不直接 `fmt.Print*` 到 stdout；优先接收 `io.Writer` / `options.Stdout()`，避免污染 MCP 协议流。
- 写文件必须检查 `Close`/`Sync`（或用 `fsutil` 原子写）。

## 9. 跨平台兼容（Windows / macOS / Linux）

发布与 CI 覆盖三个平台（`ubuntu-latest` / `macos-latest` / `windows-latest`），涉及文件系统、路径、外部命令、配置目录的代码必须三平台可编译、可测试。**不要写只在 macOS/Linux 成立的假设。**

### 9.1 路径

- 一律用 `filepath.Join` / `filepath.Clean` / `filepath.SplitList`，禁止手拼 `/` 或 `\`。
- 只在 URL、JSON、HTML、正则等协议/展示场景使用 `/`；需要给模型或日志展示路径时，可用 `filepath.ToSlash`。
- 不要假设路径以 `/` 开头；用 `filepath.IsAbs` 判断绝对路径。
- `~` 展开要同时接受 `~/` 和 `~\`，基于 `os.UserHomeDir()`，不要自己读 `$HOME`。
- Windows 路径大小写不敏感：比较路径先用 `filepath.Clean`，Windows 下再配合 `strings.EqualFold`；需要判断是否同一文件时优先 `os.SameFile`。
- 不要假设路径长度；Windows 长路径、盘符和 UNC 路径都可能出现。

### 9.2 目录与文件名

- 用户目录用 `os.UserHomeDir()`（Windows 读 `USERPROFILE`，不是 `HOME`）；配置目录沿用 `os.UserConfigDir()` / 现有 `ConfigDir()`。
- 生成文件名时过滤 Windows 非法字符 `< > : " | ? *`，并避开保留名 `CON`、`PRN`、`AUX`、`NUL`、`COM1`–`COM9`、`LPT1`–`LPT9`；同时去掉结尾的空格和点。
- 不要依赖文件名大小写区分（`a.md` 与 `A.md` 在 Windows 是同一个文件）。
- 临时文件用 `os.CreateTemp`，不要手写 `/tmp` 或 `C:\Temp`。

### 9.3 权限、符号链接、换行

- `0o755` / `0o644` 在 Windows 无实际效果，不能靠权限位判断可执行；用扩展名或 `exec.LookPath`。
- 符号链接在 Windows 可能需要开发者模式/管理员权限；`filepath.EvalSymlinks`、`os.Symlink` 失败时要有降级路径，不能直接 fatal。
- 解析文本必须兼容 CRLF；frontmatter、YAML、CSV、逐行协议先统一换行再处理。
- `os.Rename` 在 Windows 目标已存在时会失败；覆盖写要先删除或采用“临时文件 + 替换”并处理错误。
- 不要无 build tag 使用 `syscall.Stat_t`、`unix.*`、`SIGTERM` 等平台专用 API；需要时拆成 `_windows.go` / `_unix.go`。

### 9.4 外部命令

- 通过现有 `internal/agent` 的 shell 检测（bash / PowerShell）执行，不硬编码 `/bin/sh`、`grep`、`find`。
- 查找可执行文件用 `exec.LookPath`，并把 `.exe` / `.bat` / `.cmd` 纳入考虑。
- 用 `exec.Command` 的参数数组传参，不要把用户输入拼进 shell 字符串。

### 9.5 测试

- CI 在三平台跑 `go test ./...`；新增测试不得依赖平台。
- 用 `t.TempDir()` 和 `t.Chdir()`，不要写死 `/tmp`、`C:\`。
- **不要用 `t.Setenv("HOME", ...)` 隔离 home**：Windows 的 `os.UserHomeDir()` 读 `USERPROFILE`。把 home 做成可注入变量（如 `skillHomeDir`），或同时设置平台对应变量。
- 路径断言用 `filepath.Join` 构造期望值，不断言分隔符。
- 权限、符号链接、文件锁等平台差异测试用 `runtime.GOOS` skip，或放到带 build tag 的用例中。
- 比较文本时允许 CRLF：先 `strings.ReplaceAll(s, "\r\n", "\n")`。
- 测试不要依赖真实用户目录、真实 cwd 或本机已安装的技能/配置。

### 9.6 检查清单

新增或修改涉及文件系统、路径、进程、配置目录的代码后：

- [ ] `make build`
- [ ] `make test`
- [ ] `make lint`
- [ ] `make release TARGETS=windows/amd64`（至少确认 Windows 可编译）
- [ ] 测试不依赖真实 home、cwd、路径分隔符、权限位或文件名大小写
