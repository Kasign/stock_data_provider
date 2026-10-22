# Python 能力桥接服务

该模块是 Go 调用现有 Python 能力的独立边界，不是第二套分析流程。它封装当前仓库里的模块，并将返回值转换为版本化 JSON 契约；契约文件位于 `docs/architecture/python-bridge.openapi.yaml`。

## 在仓库根目录启动

```bash
export PYTHON_BRIDGE_TOKEN="$(openssl rand -hex 32)"
uvicorn python_bridge.capability_app:app --host 127.0.0.1 --port 8090
```

Docker 部署时使用可选的 `docker/docker-compose.go-migration.yml` overlay 与现有 Compose 配置组合。Bridge 不发布宿主机端口。

## 接口与鉴权

- `GET /healthz`：存活检查，不需要鉴权。
- `/internal/v1/*`：所有能力接口均要求 `X-Python-Bridge-Token`。
- 如果 token 缺失、少于 32 个字符、仍为模板占位符或与配置不一致，服务会拒绝请求。
- 每条服务边界使用独立的随机 token；Go 与 Python bridge 的配置中必须使用同一个 bridge token，不要与 Go API 的 token 混用。

Go 侧的 Python HTTP 客户端仅位于 `go-core/internal/adapters/pythonbridge/`。已有 Python 调用 Go 的场景必须使用 `go_core_client.py`，不要在多个模块中重复拼接 URL。`GO_CORE_ENABLED` 默认是 `false`，避免仅添加文件就意外改变生产流量路由。

## 兼容性边界

Bridge 包装现有接口，如 `DataFetcherManager`、`GeminiAnalyzer`、`MarketAnalyzer`、`AnalysisRepository` 和 `NotificationService`。上游模块的函数签名可能变化；在生产切换前，必须使用固定的上游版本运行 bridge 契约测试和真实 HTTP smoke test。

Bridge 是迁移阶段的兼容适配层，不代表所有 Python 能力已经迁移完成。已由 Go 原生 market-data 服务实现的行情与日 K 不应再经该 bridge 重复获取；只有尚未具备 Go 原生实现的能力才继续走 Python bridge，并须在数据来源、单位、时间和质量方面显式记录缺口。
