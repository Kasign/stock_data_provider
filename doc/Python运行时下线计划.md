# Python 运行时与代码下线计划

- 目标分支：`feat/full-go-data-providers-api-switch`
- 更新日期：2026-11-02
- 总目标：仓库最终不再包含 Python 运行时、Python 源码和 Go→Python 远程调用；国内 A 股数据服务保持 Go 原生实现。
- 本文是迁移清单，不代表 Python 已全部下线。

## 1. 明确边界

### 保留：Go 原生 Pytdx

`services/market-data/internal/provider/pytdx.go` 虽然历史名称为 Pytdx，但实现语言是 Go：

- 使用 `github.com/injoyai/tdx` / `github.com/injoyai/ios`。
- Go TCP 客户端直接实现通达信协议。
- 不调用 Python 解释器、脚本、Python bridge 或 pytdx Python 包。

因此它属于 Go Provider，继续保留并完成字段诊断。后续统一称为“Go 通达信协议 Provider”，不因名称误删。

### 必须下线：Python 运行链路

当前分支确认存在以下 Python 相关实现或依赖：

| 模块 | 当前职责/关系 | 下线要求 |
|---|---|---|
| `python_bridge/` | Python HTTP bridge，包含行情辅助、搜索、LLM、通知、持久化能力 | 能力迁移或明确废弃后整体删除 |
| `go-core/internal/adapters/pythonbridge/` | Go 分析核心调用 Python bridge 的客户端和适配器 | 用 Go 原生实现替代，删除目录 |
| `internal/adapters/pythonapi/` | 旧 Python 行情 API HTTP 客户端及采集适配 | 查清调用方后迁移/删除 |
| `cmd/pythonapi-smoke/` | 旧 Python API 连通性/数据冒烟工具 | 由 Go Provider Probe 替代后删除 |
| `docker/docker-compose.go-migration.yml` | 启动 python-bridge + go-core 的旧迁移部署配置 | Go 核心完全脱桥后删除或重写为纯 Go 部署 |
| `go-core/.env.example` | 包含 PYTHON_BRIDGE_BASE_URL / TOKEN / TIMEOUT | 移除 Python 环境变量 |
| `doc/接口规范/Python桥接.openapi.yaml` | Python bridge 接口契约 | Go API 替代完成后删除 |
| `cmd`、`scripts` 中 Python 比对/冒烟脚本 | 旧语言桥接和差分工具 | 由 Go 测试/探测替代后删除 |
| GitHub Actions Python setup | 安装 Python 并运行 Python 测试/差分 | 全部改为 Go-only workflow |
| Python/Go 迁移文档 | 旧能力盘点和迁移证据 | 迁移完成后归档或删除，不能长期保留误导性“仍需 Python”步骤 |

## 2. 当前调用依赖

当前 `go-core/cmd/analysisd/main.go` 仍强制要求：

- `PYTHON_BRIDGE_BASE_URL`
- `PYTHON_BRIDGE_TOKEN`
- `pythonbridge.Client.Health`

并通过 `pythonbridge` 适配器提供：

- 个股辅助数据、部分市场概览回退；
- 新闻搜索兼容路径；
- LLM 分析；
- 研究结果持久化；
- 通知能力。

这些是实际业务依赖，不能只删除 bridge 客户端而不替换业务端口实现。当前根目录统一部署栈未包含 Python bridge，但旧 `go-core` 迁移 Compose 仍包含它。

## 3. 分阶段执行

### 阶段 A：去掉 Python 专属数据质量工具（本轮）

- [x] 删除 Go-Python 数据质量 parity workflow，CI 不再安装 Python。
- [x] 删除 Python/Go 样本差分脚本及其 Python 单测。
- [x] 将原质量回归用例改为纯 Go 数据质量契约测试。
- [x] 保留单位、缺失值、复权、OHLC、重复日期、非有限数值和金额数量级回归保护。
- [x] 将质量约束独立成 Go 数据质量契约文档。
- [x] 记录 Pytdx 是 Go 原生协议客户端，不属于 Python 依赖。

### 阶段 B：Go 分析核心脱离 Python bridge

- [ ] 将 LLM 调用迁移为 Go 原生 Provider/统一 LLM Gateway 客户端，保留超时、重试、流式/非流式协议、模型配置和密钥隔离。
- [ ] 将搜索能力统一到现有 Go `newssearch`，移除 bridge SearchAdapter 与旧 fallback。
- [ ] 将分析结果持久化迁移到 Go repository，复用现有 MariaDB/Redis，不再通过 Python bridge 写文件或数据库。
- [ ] 将通知能力迁移为 Go 通知 Provider；若暂不需要通知，明确从分析流程移除该能力和配置。
- [ ] 将个股辅助数据、市场概览、筹码、财务、板块能力全部接到 Go market-data API；未实现的能力返回明确 unsupported/degraded，不得回退 Python。
- [ ] 移除 readiness 对 Python bridge 的依赖。
- [ ] 更新 Go Core 单测、race、vet 和集成测试。

### 阶段 C：删除 Python 代码与部署

- [ ] 删除 `python_bridge/`、Python 依赖清单、Python Dockerfile 和 Python 专属测试。
- [ ] 删除 `go-core/internal/adapters/pythonbridge/`。
- [ ] 清理 `internal/adapters/pythonapi/` 和 `cmd/pythonapi-smoke/`，以引用扫描确认没有调用方。
- [ ] 删除 Python bridge OpenAPI、旧 migration Compose、Python 环境变量和部署说明。
- [ ] 清理 GitHub Actions 中 Python setup、pip install、python unittest 和 parity workflow。
- [ ] 全仓扫描 `python`、`PYTHON_`、`python3`、`pip`、`uvicorn`、`requirements.txt`，确认仅剩历史说明或无关命名。
- [ ] Go-only Docker 镜像构建、Compose 校验、测试、race、vet 和部署验收全部通过。

## 4. 执行顺序与限制

1. 优先完成 Go 分析核心能力迁移，避免业务功能被删除。
2. 数据层继续只做 Go Provider；不要再新增 Python Provider 或 Python 辅助脚本。
3. 不因清理 Python 而弱化 Go 数据质量契约（见 `doc/Go数据质量契约.md`）。
4. 每完成一个能力域，先移除调用与配置，再删除 Python 实现；按完整功能批次提交。
5. 阶段 C 完成前，不得宣称“Python 已彻底下线”。
6. Pytdx 的 Go 协议实现不属于下线对象。

## 5. 当前状态

- Python 质量对照 CI 和 Python 样本差分工具：本轮计划移除。
- Go 原生行情数据层：继续保留。
- Python bridge：仍被旧 Go Core 业务流程引用，暂不能安全删除。
- 下一批优先：检查 Go Core 的 LLM / persistence / notification 端口，设计脱桥实现；同时继续完成 Go Provider 字段诊断。


### 阶段 D：Go 通达信协议 Provider 字段诊断（2026-11-02）

- [x] 保留 Go 原生 TCP 通达信协议实现，不依赖 Python。
- [x] 对日 K 时间戳、重复日期、OHLC、成交量、成交额与成交量零值不一致增加结构化 Parse Issue。
- [x] 诊断保留协议解码后的行号，不回显原始二进制报文。
- [x] 对无前收盘价无法计算涨跌幅的情况记录降级诊断。
- [x] 增加纯 Go 单测覆盖异常价格与金额/成交量不一致。
- [ ] 后续若协议库暴露字段级解码错误，应再评估协议层原始字节诊断；当前库返回强类型 Kline，无法恢复已在二进制解码阶段丢弃的原始字段字节。
