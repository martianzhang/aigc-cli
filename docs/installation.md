# 安装与配置

## 安装

### go install

```bash
go install github.com/martianzhang/apimart-cli@latest
```

### 从源码构建

```bash
git clone https://github.com/martianzhang/apimart-cli.git
cd apimart-cli
make build
```

### Makefile 常用命令

```bash
make          # 编译
make run ARGS="image --help"   # 运行查看帮助
make clean    # 清理产物
make lint     # 静态检查
make test     # 运行测试
make cover    # 测试覆盖率
make release  # 交叉编译（所有平台）
```

## 配置 API Key

三种设置方式（优先级从高到低）：

```bash
# 方式一：命令行参数
apimart-cli image --prompt "..." --api-key "sk-xxx"

# 方式二：环境变量（支持两套命名）
export OPENAI_API_KEY="sk-xxx"
# 或兼容旧的：
# export APIMART_API_KEY="sk-xxx"

# 方式三：配置文件
```

## 配置文件

支持两个位置（`~/.config/openai/config.yaml` 优先，`~/.config/apimart/config.yaml` 回退）：

```yaml
api_key: "sk-xxx"

# API 地址（默认 https://api.apimart.ai）
# base_url: "https://api.apimart.ai"

# 生成模式：auto（自动检测）、sync（同步）、async（异步任务）
# 默认 auto，会根据 base_url 自动识别
# mode: "auto"

# HTTP 代理
# 也可通过 OPENAI_HTTP_PROXY 或 APIMART_HTTP_PROXY 环境变量设置
http_proxy: "http://127.0.0.1:7890"

# 图片下载目录（默认当前目录 "."）
# output_dir: "./downloads"

# 全局超时（秒，默认 180），各模态 defaults.*.timeout 会覆盖此值
# timeout: 300

defaults:
  image:
    model: "gpt-image-2-official"
    size: "3:1"
    resolution: "1k"
    quality: "low"
    output_format: "png"
    # timeout: 300                 # HTTP 超时秒数（覆盖 timeout）

  video:
    model: "doubao-seedance-2.0"
    # timeout: 600                 # HTTP 超时秒数（视频生成更慢）
```

完整示例见 [config.example.yaml](config.example.yaml)。

### 提示词文件

加 `--save-prompt` 可将提示词保存到 `image_{task_id}.md` 文件，方便追溯：

```bash
apimart-cli image --prompt "A red fox" --save-prompt
```

## 代理配置

```bash
# 命令行指定
apimart-cli image --prompt "..." --http-proxy "http://127.0.0.1:7890"

# 环境变量（支持 HTTP_PROXY / HTTPS_PROXY / ALL_PROXY / NO_PROXY）
export HTTP_PROXY="http://127.0.0.1:7890"

# SOCKS5
apimart-cli image --prompt "..." --http-proxy "socks5://127.0.0.1:1080"
```

## 优先级规则

**CLI 参数 > JSON 输入 > YAML 配置 > 代码默认值**

代理优先级：
**`--http-proxy` 参数 > `OPENAI_HTTP_PROXY` / `APIMART_HTTP_PROXY` 环境变量 > `HTTP_PROXY` 标准环境变量**
