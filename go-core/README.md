# Go 分析核心服务（迁移阶段一）

这是一个独立的 Go module。所有 Go 源码都位于 `go-core/` 下，不会放进现有 Python 项目的 `src/`、`api/` 或 `data_provider/` 目录。

## 当前包含的能力

- 为单股分析和市场复盘提供带鉴权的内部 HTTP 接口。
- 由 Go 管理主流程：获取数据、计算确定性技术指标、建立上下文和质量摘要、调用分析、持久化，并可选发送通知。
- Python 能力适配器集中放在 `internal/adapters/pythonbridge`。
- Go 不直接引入 ORM 或 pandas；现阶段 Python 仍拥有部分存储 schema 和 Provider SDK。
- 工作流返回分阶段 trace，包含每一步的状态与耗时，供调用方查看完整执行过程。

## 测试与构建

```bash
cd go-core
gofmt -w ./cmd ./internal
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/analysisd
```

## 必需环境变量

- `GO_CORE_TOKEN`：访问 `/internal/v1/analysis/*` 所需的非占位长密钥，至少 32 个字符，可用 `openssl rand -hex 32` 生成。
- `PYTHON_BRIDGE_TOKEN`：调用 Python bridge 所需的非占位长密钥，至少 32 个字符。
- `PYTHON_BRIDGE_BASE_URL`：默认 `http://python-bridge:8090`。
- `HTTP_ADDR`：默认 `127.0.0.1:8088`；Docker Compose 会显式覆盖为容器网络监听地址 `:8088`。
- `GO_CORE_REQUEST_TIMEOUT` 和 `PYTHON_BRIDGE_TIMEOUT`：使用 Go duration 格式，如 `20m`。

## 接口

- `GET /healthz`：存活检查。
- `GET /ready`：依赖服务就绪检查。
- `GET /internal/v1/capabilities`：列出工作流能力。
- `POST /internal/v1/analysis/stock`：分析一只股票（兼容接口）。
- `POST /internal/v1/analysis/stocks`：批量分析股票。
- `POST /internal/v1/analysis/market-review`：区域市场复盘。

该服务必须只暴露在内部 Docker 网络或绑定到回环地址的端口上，不要直接发布到公网。

## 当前切换边界

本代码不会自动替换原有的 `main.py`、FastAPI 路由、Bot 或调度器。只有兼容性测试和 golden tests 通过后，现有入口才能经由 `python_bridge/go_core_client.py` 切换到 Go；否则可能导致新旧流程同时执行同一个任务。

当前版本仍复用 Python 的 `GeminiAnalyzer`、部分原始数据源层、旧持久化 schema 和通知实现。分析上下文的完整行为一致性仍需在替换生产 `StockAnalysisPipeline` 前补齐。个股行情与日 K 已有原生 Go market-data HTTP 接口，但市场概览、新闻搜索、LLM、部分持久化与通知能力仍走 bridge。参见仓库根目录的迁移方案和 Provider 迁移矩阵。
