# AGENTS.md — AI 助手工作指南

> 本文档面向 AI 编码助手（Claude / OpenCode / Cursor 等），定义在本项目中工作时的约束和流程。
> 详尽的使用文档见 `docs/zh/` 与 `docs/en/`（中文优先），本文档只保留**开发约束**，不重复维护用户文档内容。

## 外部文件加载

本文件中的 `@路径` 引用需要按需读取（opencode 不会自动解析）：

- **代码规范** `SPEC.md`：已通过 `.opencode/opencode.json` 的 `instructions` 自动加载，**写代码时默认生效**，无需手动读取。
- **项目结构** `@CONTRIBUTING.md`：涉及架构/目录理解时用 Read 工具读取。
- **用户文档** `@docs/...`：涉及对应命令的功能细节时用 Read 工具读取（它们是你需要同步维护的用户文档，勿在 AGENTS 中重复声明）。

**不要**预先加载所有引用，只在任务确实相关时按需读取。

---

## 一、构建系统（唯一入口）

所有构建、格式化、测试、覆盖率均通过 **Makefile** 管理，**禁止直接调用 `go build` / `go test` / `go fmt`**。

| 命令 | 作用 | 必须运行 |
|---|---|---|
| `make fmt` | `go fmt ./...` 格式化代码 | 每次编辑后 |
| `make build` | 编译二进制 | 每次编辑后 |
| `make lint` | `go vet ./...` + `golangci-lint` 静态检查 | 每次提交前 |
| `make test` | 运行全部测试 | 每次编辑后 |
| `make cover` | 测试覆盖率报告 | 每次提交前 |
| `make run ARGS="..."` | 编译并运行 | 手动验证时 |
| `make release` | 跨平台交叉编译 | CI 自动执行 |

### 1.1 本地原生库依赖模式（无 CGO）

本地能力（TTS/ASR、ONNX 推理、图片编解码）依赖原生 C/C++ 库，但**全部通过纯 Go 接入，Go 源码零 `import "C"`**。两条路径，新增原生库依赖时必须遵守：

1. **动态库运行时加载**（sherpa-onnx / onnxruntime）：通过 **purego**（dlopen）在运行时加载，配合编译期生成的 C helper。helper 定义在 `scripts/helper.c`，由 `scripts/build-helper.sh` 编译；运行库由 `init` 命令下载到 `~/.config/aigc-cli/models/`。结构见 `internal/audio/`（`tts.go` / `tts_sherpa.go` / `sherpa_ffi.go` / `loadlib.go`）。
2. **wasm 编解码**（jpegli / libwebp / libavif / libjxl）：通过 **gen2brain 系列库**接入，底层 codec 编译为 wasm 并 `go:embed`，由 **wazero**（纯 Go wasm 运行时）执行，零 CGO、零外部文件。编码统一走 `internal/imgcodec.EncodeToFile`。

**依赖锁定**：`go mod tidy` 会删除"不被 Go 代码 import"的模块——仅被编译期使用的模块（如 sherpa 平台模块）必须通过 `internal/tools/tools.go`（`//go:build tools`）空导入锁定，**禁止**手动改 go.mod 维护。若某库只能 CGO，需提供 stub 实现并合入前评审。

---

## 二、开发工作流（强制）

每次修改代码后必须按此顺序：

```
代码修改 → make fmt → make build → make test → 同步文档
```

- 遵循 Go 标准风格，提交前必须 `make fmt`
- 编译产物在项目根目录（`aigc-cli` 或 `aigc-cli.exe`）
- 测试失败时：先确认是否**已有失败**（`git stash` 后对比），自己引入的修复，已有的在变更中注明
- 覆盖率关注变更文件趋势，非硬性门槛

### 2.1 调试原则

优先使用 CLI 现有诊断开关（`-v/--verbose`、`--dry-run` 打印等价 curl、既有日志），**不要临时插入 `fmt.Println`**——诊断开关是产品能力，无需清理；临时埋点提交前务必删除或用 `// TODO(debug)` 标记。

---

## 三、文档同步规则（⚡ 硬性要求）

**文档不得滞后于代码。** 任何功能性变更（新增/修改命令、参数、行为）都必须同步更新对应文档（`docs/zh/` 与 `docs/en/` 都要改）：

| 变更 | 同步文档 |
|---|---|
| 新增/删除命令、修改用法、新增依赖 | `README.md` |
| 修改安装方式、环境变量、配置路径 | `docs/*/installation.md` |
| 修改 `image`/`video`/`chat`/`detect`/`midjourney`/`ideas`/`audio` 命令 | 对应 `docs/*/guide-*.md` |
| 修改 `models`/`task`/`balance`/`dry-run` 等辅助命令 | `docs/*/guide-commands.md` |
| 修改 MCP 工具定义 | `docs/*/guide-mcp.md` |
| 新增/修改配置字段 | `docs/*/config.example.yaml` |
| 新增常见问题 | `docs/*/faq.md` |
| 每次发版前 | `docs/release_notes/vX.Y.Z.md` |

注意事项：
- 文档写**用户视角**，示例命令必须真实可运行
- 新增 flag 要同时在 `--help` 和对应 guide 中体现
- **例外**：纯重构、修 typo、内部测试变更无需更新文档，但 commit message 注明 `(no-doc)`

---

## 四、代码规范

> 完整代码规范（导入顺序、错误处理、SilenceUsage、命名、文件规模、共享工具、配置访问器）见 @SPEC.md。

**核心红线**（详见 @SPEC.md）：
- 配置访问一律用访问器（`chatDefaults()` / `imageDefaults()` 等），**禁止手写 `shared.Cfg != nil && shared.Cfg.Defaults != nil && ...` 长链 nil 判断**
- 所有有 `RunE` 的 cobra 命令必须 `SilenceUsage: true`
- 错误包装用 `%w`，错误消息首字母小写
- 提交信息格式：`<type>(<scope>): <描述>`（type: feat/fix/refactor/docs/test/chore/style）

---

## 五、项目架构与关键设计

项目目录结构见 @CONTRIBUTING.md（单一来源）。

### 5.1 HTTP 代理（http_proxy）

配置了 `http_proxy` 后，**所有** HTTP 请求都必须走代理（API 调用、文件下载、非 API 请求）。在启动入口（`root.go` / `mcp.go` 的 `PersistentPreRunE`）调用 `client.ConfigureDefaultClient(proxyURL)` 配置全局 `http.DefaultClient`。新增 HTTP 调用时**优先使用 `http.DefaultClient`**，不要新建 `http.Client` 或自行配置 `Transport`。

### 5.2 关键设计决策

- **Provider 检测**集中到 `internal/provider`，新增 provider 只改此包和策略表
- **策略路由**（`imageStrategies` / `videoStrategies`）用 match-run 模式派发
- **命名 Provider**：配置 `providers`，各命令通过 `defaults.{cmd}.provider` 引用
- **Provider 优先级链**：`--api-key/--api-base` > `--provider` > `defaults.{cmd}.provider` > 全局 `api_key/base_url` > 代码默认
- **`shared.ResolveProvider(cmdName)`**：获取 `*provider.EffectiveProvider`，代替直接访问 `shared.APIKey`/`shared.APIBase`
- **`client.NewFromProvider(p)`**：通过 provider 创建客户端
- **内置 `local` Provider**：`--provider local` 自动路由到本地 ONNX
- **在线/离线分支**：ocr/vision/detect/background/audio 未指定 provider 时默认本地 ONNX，指定了则走在线
- **`internal/provider/online.go`**：在线 LLM 调用（`OCRImage()`/`DescribeImage()`）
- **文件上传**在 client 层自动处理本地路径→URL
- **配置文件**：`~/.config/aigc-cli/config.yaml`

---

## 六、水印流程

完整的水印学习 / 去除 / 添加流程见 @docs/zh/guide-watermark.md（两拍法：黑底+灰底种子图 → `--learn-watermark` 求解 alpha map）。

---

## 七、测试策略

大部分 API（图片/视频/对话）是**付费接口**，无法在 CI 无成本调用：

- **必须覆盖**（无成本）：配置加载合并、Provider 检测、类型序列化、CLI 参数解析校验、HTTP 请求构建与 curl 生成、无外部依赖的纯函数
- **逐步 mock**：Client 请求/响应、MCP handler、命令完整执行路径
- **新增代码原则**：纯函数写表驱动测试；重构优先提取可测纯函数；mock 优先于集成测试

---

## 八、禁止行为

| 禁止事项 | 说明 |
|---|---|
| ❌ 直接调用 `go build` / `go test` / `go fmt` | 必须走 Makefile |
| ❌ 修改后不跑 `make build` / `make test` | 必须确保编译与测试通过 |
| ❌ 功能变更不同步文档 | 文档不得滞后代码 |
| ❌ 抑制类型检查 / 吞错误 | 不用 `as any`/`@ts-ignore`；不写空 catch；错误用 `%w` 包装 |
| ❌ 主动 commit | 除非用户明确要求 "commit"，否则不提交；讨论阶段先暂存 |
| ❌ 提交前不检查 LSP 诊断 | 确保新增代码无警告 |
