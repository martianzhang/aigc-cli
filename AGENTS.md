# AGENTS.md — AI 助手工作指南

> 本文档面向 AI 编码助手（Claude / OpenCode / Cursor 等），定义在本项目中工作时的约束和流程。

---

## 一、构建系统

本项目的所有构建、格式化、测试、覆盖率均通过 **Makefile** 管理，**禁止直接调用 `go build`、`go test` 等命令**。

| 命令 | 作用 | 必须运行 |
|---|---|---|
| `make fmt` | `go fmt ./...` 格式化代码 | 每次编辑后 |
| `make build` | 编译二进制 | 每次编辑后 |
| `make lint` | `go vet ./...` 静态检查 | 每次提交前 |
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

编译必须通过。编译产物在项目根目录（`apimart-cli.exe` 或 `apimart-cli`）。

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
| `docs/guide-midjourney.md` | 修改 `midjourney` 命令参数或行为 |
| `docs/guide-commands.md` | 修改 `models`/`task`/`balance`/`dry-run` 等辅助命令 |
| `docs/faq.md` | 新增常见问题 |
| `docs/mcp.md` | 修改 MCP 工具定义或配置方式 |
| `config.example.yaml` | 新增/修改配置字段 |

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

### 4.3 变量命名

- Go 驼峰式：`APIKey`、`HTTPProxy`、`baseURL`
- 不要用拼音命名，不要用单字母变量（循环变量除外）

### 4.4 配置优先级

```
CLI 参数 > JSON 输入 > YAML 配置 > 代码默认值
```

修改配置相关代码时，务必维护此优先级。

### 4.5 提交信息

```
<type>(<scope>): <简短描述>

<可选详细描述>
```

type: `feat` / `fix` / `refactor` / `docs` / `test` / `chore` / `style`
scope: `image` / `video` / `chat` / `midjourney` / `mcp` / `config` / `docs` / `skill`

---

## 五、项目架构

```
apimart-cli/
├── cmd/              # cobra 命令定义（薄层：解析参数→调用逻辑→输出结果）
├── internal/
│   ├── client/       # HTTP API 客户端（APIMart / OpenAI / OpenRouter / 云坞）
│   ├── config/       # Viper 配置加载（YAML + 环境变量）
│   ├── mcp/          # MCP Server 实现
│   ├── provider/     # Provider 检测（APIMart / OpenAI / OpenRouter）
│   └── types/        # 请求/响应数据结构和配置类型
├── docs/             # 用户文档
├── skills/           # AI Agent SKILL 定义
├── main.go           # 入口
├── Makefile          # 统一构建入口
├── AGENTS.md         # ← 当前文件
└── TODO.md           # 已知问题清单
```

### 5.1 关键设计决策

- **Provider 检测**集中到 `internal/provider`，新增 provider 只需改此包和策略表
- **策略路由**（`imageStrategies` / `videoStrategies`）用 match-run 模式派发到不同后端
- **文件上传**在 client 层自动处理本地路径→URL 转换
- **配置文件**兼容 `~/.config/openai/config.yaml` 和 `~/.config/apimart/config.yaml`

### 5.2 已知技术债务

详见 [TODO.md](TODO.md)，当前重点关注：

- `cmd/midjourney.go` 的 `newMJClient()` 递归调用（P0）
- MCP handler 返回 `nil, error`（P0）
- `handleSSE` 的 `os.Stdout` 耦合（P1）
- 全局变量传参模式（P1）

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

## 七、禁止行为

| 禁止事项 | 说明 |
|---|---|
| ❌ 直接调用 `go build` / `go test` / `go fmt` | 必须走 Makefile |
| ❌ 修改代码后不跑 `make build` | 必须确保编译通过 |
| ❌ 修改代码后不跑 `make test` | 必须确保测试通过 |
| ❌ 功能变更不同步文档 | 文档不得滞后代码 |
| ❌ 使用 `as any` / `@ts-ignore` | Go 没有，但任何时候不要抑制类型检查 |
| ❌ 多余的空 `catch` 块 | Go 中没有 try-catch，但不要吞错误 |
| ❌ 提交前不检查变更文件的 LSP 诊断 | 确保新增代码无警告 |
