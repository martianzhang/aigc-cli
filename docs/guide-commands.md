# 其他命令

## 查询模型列表

支持两个数据源，自动根据 API 地址选择：

| base_url | 数据源 | 特点 |
|---|---|---|
| APIMart 域名 | `GET /api/marketplace/models` | 按类型筛选、定价信息、免认证 |
| 其他 | `GET /v1/models` | OpenAI 标准格式 |

```bash
# APIMart 市场（所有模型）
apimart-cli models

# APIMart 市场（按类型筛选）
apimart-cli models image
apimart-cli models video
apimart-cli models chat
apimart-cli models --type image

# APIMart 特定模型定价
apimart-cli models --price gpt-image-2-official
apimart-cli models --price doubao-seedance-2.0

# OpenAI / OpenRouter 标准模型列表
apimart-cli models --base-url "https://openrouter.ai/api/v1"
```

## 查询任务状态

仅 APIMart 异步模式可用：

```bash
apimart-cli task task_01KV4KD9FBH3AZ4DE18A7Y17S3
```

返回完整的任务信息（状态、进度、耗时、费用、结果 URL 等）。图片任务完成后自动下载图片到 `--output` 目录。

## 查询余额

仅 APIMart 可用：

```bash
# 查询当前 API Key（Token）的余额
apimart-cli balance

# 查询用户账号的总余额
apimart-cli balance user
```

## Dry-run 调试

打印即将提交的 curl 命令，不实际调用 API：

```bash
# 图片 dry-run
apimart-cli image --prompt "test" --size "16:9" --dry-run

# 视频 dry-run
apimart-cli video --prompt "test" --duration 4 --dry-run

# Midjourney dry-run
apimart-cli mj imagine --prompt "test" --dry-run
apimart-cli mj upscale --task-id task_xxx --index 1 --dry-run
```

## 查看版本

```bash
apimart-cli version
# 或
apimart-cli --version
```

## API 参考

| 端点 | 用途 | 适用 |
|---|---|---|
| `POST /v1/chat/completions` | AI 对话 | 通用 ✅ |
| `POST /v1/images/generations` | 文生图（同步/异步） | 通用 ✅ |
| `POST /v1/videos/generations` | 文生视频 | APIMart ✅ |
| `POST /v1/midjourney/generations` (及 16 个子端点) | Midjourney 图生/编辑 | APIMart ✅ |
| `POST /v1/uploads/images` | 上传图片 | APIMart ✅ |
| `GET /v1/tasks/{task_id}` | 查询任务状态 | APIMart ✅ |
| `GET /v1/midjourney/{task_id}` | 查询 MJ 任务（含 buttons） | APIMart ✅ |
| `GET /v1/balance` | Token 余额查询 | APIMart ✅ |
| `GET /v1/user/balance` | 用户余额查询 | APIMart ✅ |
| `GET /api/marketplace/models` | 模型列表（免认证） | APIMart ✅ |
| `GET /api/pricing/model` | 模型定价详情（免认证） | APIMart ✅ |
| `GET /v1/models` | 模型列表 | OpenAI/OpenRouter ✅ |

完整文档见 [docs.apimart.ai](https://docs.apimart.ai/en)。
