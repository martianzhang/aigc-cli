# 贡献指南

感谢你考虑为 aigc-cli 贡献代码！本文档帮助你快速上手。

## 开发环境

- Go 1.26+（见 [go.mod](go.mod)；构建使用 toolchain go1.27.4）
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
├── cmd/              # cobra wiring（通常只有 rootCmd.AddCommand）
│   ├── root.go       # 入口、PersistentPreRunE、全局 flags、ExecuteContext
│   ├── config.go     # config get/set/list
│   ├── image.go      # 一行：rootCmd.AddCommand(image.Cmd())
│   └── ...
├── internal/
│   ├── cli/<cmd>/    # 命令实现：flag 解析 + 输出；image/video/chat/...
│   ├── cli/options/  # Shared 状态、配置访问器、client factory、flag 工具
│   ├── client/       # HTTP API 客户端（APIMart / OpenAI / OpenRouter 等）
│   ├── config/       # YAML 配置加载（viper）+ 密钥加密
│   ├── provider/     # Provider 检测 + 命名 Provider 解析 + 在线 LLM 调用
│   ├── service/      # 下载、输入解析、图片/视频落盘等共享业务逻辑
│   ├── httpx/        # 重试、Retry-After、响应体大小上限
│   ├── modelstore/   # 模型下载、Range 续传、SHA256 校验
│   ├── imgcodec/     # 图片编解码（wasm codec）+ 解码炸弹上限
│   ├── argutil/      # ffmpeg 风格参数拆分
│   ├── wait/         # context-aware 休眠
│   ├── mcp/          # MCP Server 实现
│   └── types/        # 请求/响应数据结构和配置类型
├── docs/             # 用户文档（zh/ 与 en/，文件集合需一致）
│   └── release_notes/ # 各版本 release notes
├── scripts/          # 辅助脚本（helper.c / build-helper.sh / check_docs.py）
├── main.go           # 入口
├── Makefile          # 统一构建入口
└── AGENTS.md         # AI 助手开发约束（引用 SPEC.md）
```

## 常用命令

```bash
make build    # 编译
make test     # 运行测试
make lint     # go vet 静态检查
make cover    # 测试覆盖率
make bench    # 运行基准测试（可用 BENCH=ClipString 过滤）
make clean    # 清理产物

# 快速运行
make run ARGS="image --help"
make run ARGS="chat --message hello"
```

## 开发规范

> 完整的代码规范（导入顺序、错误处理、SilenceUsage、命名、配置访问器、提交信息、文件规模、共享工具）见 [SPEC.md](SPEC.md)，本文档只保留贡献流程要点。

### 代码风格

- 使用 `go fmt ./...`（`make fmt`）自动格式化
- 使用 `go vet ./...`（`make lint`）检查常见问题
- 导入按标准库 → 第三方 → 内部包分组，组间空行分隔
- 错误用 `%w` 包装，首字母小写
- 配置访问用访问器（`chatDefaults()` 等），禁止手写长链 nil 判断
- 提交信息格式 `<type>(<scope>): <描述>`

### 测试

- **有状态的逻辑**（API 调用等）使用 mock 或 interface 隔离
- **纯函数**（序列化、校验、格式转换）优先写表驱动测试
- MCP handler 添加单元测试，mock HTTP client
- 提交前确保 `make test` 通过

## PR 流程

1. Fork 仓库并创建你的 feature branch
2. 遵循上述代码规范和提交信息格式
3. 确保 `make lint && make test` 通过
4. 如果新增了 CLI 命令，同步更新 docs/ 目录
5. 发起 PR 到 `main` 分支，描述变更内容

## Issue 报告

报告 bug 时请提供：

- aigc-cli 版本（`aigc-cli version`）
- 操作系统
- 完整的命令和输出（注意隐去 API Key）
- 期望行为和实际行为

## License

贡献的代码将遵循 [MIT License](LICENSE)。
