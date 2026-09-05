# SPEC.md — 代码规范

> 被 @AGENTS.md 引用（并已通过 opencode.json 自动加载）。本文档定义写代码时必须遵守的 Go 编码规范。

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
- 错误消息首字母小写
- CLI 层返回 error，由 `cmd.Execute()` 统一处理
- 不写空 `catch` 块 / 不吞错误

## 3. SilenceUsage

所有有 `RunE` 的 cobra 命令**必须**设置 `SilenceUsage: true`。因为 `RunE` 返回的错误通常是运行时错误（API 调用失败、网络超时等），不是参数解析错误。显示 Usage 会干扰用户查看真正的错误信息。

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

## 5. 配置访问器（⚡ 新增，防长链 nil 判断）

**禁止**手写 `shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Chat != nil && ...` 这类逐级 nil 判断。

一律通过访问器 / 泛型工具获取：

```go
// 按 section 访问器（已定义于 cmd/config_defaults.go）
cfg := chatDefaults()        // *types.ChatDefaults，nil-safe
cfg := imageDefaults()       // *types.ImageDefaults
cfg := audioDefaults()       // *types.AudioDefaults
cfg := knowledgeDefaults()   // *types.KBDefaults
cfg := detectConfig()        // *types.DetectConfig
```

访问器返回 nil 时再判空：

```go
if cfg := chatDefaults(); cfg != nil && cfg.Model != "" {
	shared.Model = cfg.Model
}
```

新增配置 section 时，在 `cmd/config_defaults.go` 用 `field` 组合添加访问器：

```go
func myDefaults() *types.MyDefaults {
	return field(defaultsOrNil(), func(d *types.ConfigDefaults) *types.MyDefaults { return d.My })
}
```

通用工具（`cmd/util_ptr.go`）：
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

### 8.1 文件规模

单个 `.go` 文件尽量控制在 200 行以内（不含空白行和注释）。超过后建议按功能域拆分到独立文件，便于 AI 分析。拆分时保持 `package cmd` 不变，不修改任何代码逻辑。

示例：`cmd/image.go`（823 行 → 5 个文件）

| 文件 | 职责 |
|---|---|
| `image.go` | 命令定义、`runImageGenerate`、标志注册、`init` |
| `image_request.go` | 请求构建（`buildImageRequest`、`buildImageCurl`）和输入解析 |
| `image_dispatch.go` | 策略上下文/类型/路由表 |
| `image_runners.go` | 各 Provider 执行函数和 `downloadImages` |
| `image_helpers.go` | Image 模块专有辅助函数 |

同样，`cmd/video.go`（901 行 → 5 个文件）：`video.go` / `video_request.go` / `video_dispatch.go` / `video_runners.go` / `video_helpers.go`。

### 8.2 共享工具

跨模块复用的公共函数放在 `cmd/util.go` 中，不挂靠在任何一个命令文件上。当前已提取的共享函数：

| 函数 | 调用方 | 职责 |
|---|---|---|
| `readInput` | image / video / midjourney / chat | 从文件、stdin 或字符串读取输入 |
| `isFile` | image / video / midjourney | 判断路径是否为已有文件 |
| `httpGet` | image / video | HTTP GET 或 data URI 转二进制 |
| `applyTimeout` | image / video / midjourney | 按优先级设置 HTTP 客户端超时 |
| `setIntFlag` | video / chat | 按 cobra flag 是否变更设置 `*int` 字段 |
| `setBoolFlag` | video / chat | 按 cobra flag 是否变更设置 `*bool` 字段 |
| `downloadVideos` | video / midjourney / task | 下载生成的视频列表到输出目录 |
| `field` / `deref` | 全命令 | 泛型安全导航 + 指针安全取值（`util_ptr.go`） |
| `chatDefaults()` 等 | 全命令 | 配置 section 访问器（`config_defaults.go`） |

**规则**：新增的跨命令公共函数必须放在 `cmd/util.go`（或 `util_ptr.go` / `config_defaults.go`）中，不要放在某个命令的专属文件（如 `image_*.go`）里。如果某个函数理论上可以独立存在、不依赖具体命令的业务逻辑，它就是共享工具函数。
