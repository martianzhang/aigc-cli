# 贡献指南

感谢你考虑为 aigc-cli 贡献代码！本文档帮助你快速上手。

## 开发环境

- Go 1.25+（见 [go.mod](go.mod)）
- 支持 macOS、Linux、Windows

```bash
# 克隆
git clone https://github.com/martianzhang/aigc-cli.git
cd aigc-cli

# 编译
make build

# 验证
./aigc-cli version
```

> 建议在开发时设置 `OPENAI_API_KEY` 环境变量，或按 [docs/installation.md](docs/installation.md) 创建配置文件。

## 项目结构

```
aigc-cli/
├── cmd/              # CLI 命令定义（cobra）
│   ├── root.go       # 根命令、全局 flag、配置加载
│   ├── image.go      # 图片生成
│   ├── video.go      # 视频生成
│   ├── chat.go       # AI 对话
│   ├── midjourney.go # Midjourney 生成/编辑
│   ├── models.go     # 模型列表 & 定价
│   ├── task.go       # 任务查询
│   ├── balance.go    # 余额查询
│   ├── mcp.go        # MCP Server 入口
│   └── version.go    # 版本信息
├── internal/
│   ├── client/       # HTTP API 客户端
│   ├── config/       # YAML 配置加载（viper）
│   ├── mcp/          # MCP Server 实现
│   └── types/        # 请求/响应数据结构
├── docs/             # 用户文档
├── skills/           # AI Agent SKILL 定义
├── main.go           # 入口
└── Makefile          # 构建、测试、发布
```

## 常用命令

```bash
make build    # 编译
make test     # 运行测试
make lint     # go vet 静态检查
make cover    # 测试覆盖率
make clean    # 清理产物

# 快速运行
make run ARGS="image --help"
make run ARGS="chat --message hello"
```

## 开发规范

### 代码风格

- 使用 `go fmt ./...`（`make fmt`）自动格式化
- 使用 `go vet ./...`（`make lint`）检查常见问题
- 导入按标准库 → 第三方 → 内部包分组，组间空行分隔

### 错误处理

- 使用 `fmt.Errorf("context: %w", err)` 包装错误（`%w`，不是 `%v`）
- 有意义的错误消息，首字母小写
- CLI 错误统一返回 error，由 `cmd.Execute()` 打印到 stderr

### 命名

- 遵循 Go 惯例：驼峰式，缩写全大写（`APIKey`、`HTTPProxy`）
- 包名小写、单数（`client`、`config`、`types`）
- 测试函数：`TestXxx`，表驱动测试优先

### 配置优先级

CLI 参数 > JSON 输入 > YAML 配置 > 代码默认值

添加新 flag 时，确保按此优先级处理。

### 提交信息

```
<type>(<scope>): <简短描述>

<可选详细描述>
```

type: feat / fix / refactor / docs / test / chore / style
scope: image / video / chat / midjourney / mcp / config / docs / skill

示例：
```
feat(image): 支持 Grok Imagine 1.5 Edit
fix(mcp): 使用配置文件中的 output_dir
docs(image): 补充 --edit 模式说明
```

### 测试

- **有状态的逻辑**（API 调用等）使用 mock 或 interface 隔离
- **纯函数**（序列化、校验、格式转换）优先写表驱动测试
- MCP handler 添加单元测试，mock HTTP client
- 提交前确保 `make test` 通过

## PR 流程

1. Fork 仓库并创建你的 feature branch
2. 遵循上述代码规范和提交信息格式
3. 确保 `make lint && make test` 通过
4. 如果新增了 CLI 命令，同步更新 docs/ 和 skills/ 目录
5. 发起 PR 到 `main` 分支，描述变更内容

## Issue 报告

报告 bug 时请提供：

- aigc-cli 版本（`aigc-cli version`）
- 操作系统
- 完整的命令和输出（注意隐去 API Key）
- 期望行为和实际行为

## License

贡献的代码将遵循 [MIT License](LICENSE)。
