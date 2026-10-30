# Go 数据服务迁移状态

- 适用分支：`feat/full-go-data-providers-api-switch`
- 静态盘点基线：`4c603172448102058f95e6bc625494436a8402c9`
- 更新日期：2026-10-30
- 状态口径：只记录当前代码/配置可证明的状态，不把静态实现当作真实生产可用。

## 1. 总体结论

Go 原生行情服务已覆盖国内 A 股多个主要数据域，具有独立 Provider 接口、按能力注册、配置开关、服务层回退、数据质量归一化、缓存和 MariaDB 持久化。

当前不能宣布 Python → Go 全量迁移完成，原因包括：

1. 原 Python DataFetcherManager、Efinance、AkShare 的完整源码和固定版本未纳入当前仓库，尚不能逐函数还原真实旧调用链。
2. 当前只完成了代码/配置静态盘点，尚未完成所有数据域的同样本字段差分。
3. 部分 Go Provider 代码已实现但默认关闭，权限、限流和字段覆盖尚未统一验收。
4. 严格 PIT、部分 Provider 的真实权限验收和固定样本字段对账仍有明确缺口。
5. Python bridge、旧 Python API 适配器和 Go Core 兼容桥仍保留，不能据此认定 Python 运行时已全面下线。

## 2. 当前实现基线

| 层级 | 代码位置 | 当前判断 |
|---|---|---|
| Go 行情服务入口 | `services/market-data/cmd/market-data/main.go` | Provider 实例化、按配置启用、按能力接口注册 |
| Provider 接口 | `services/market-data/internal/provider/provider.go` | 已拆分 quote、daily bars、财务、估值、资金流、筹码、指数等独立能力 |
| Provider 配置 | `services/market-data/configs/providers.json`、`services/market-data/internal/config/config.go` | 支持 Provider 优先级和逐能力开关；妙想为独立补充源配置 |
| 服务层 | `services/market-data/internal/service` | 已有多数据域服务和 fallback 测试 |
| HTTP API | `services/market-data/internal/httpapi`、`services/market-data/openapi.yaml` | 已提供主要行情 API；以路由、Handler、OpenAPI 联合确认能力 |
| 质量归一 | `services/market-data/internal/normalize`、各 Provider 实现 | 已有单位、复权、空值、数值边界及质量状态保护 |
| Python bridge | `python_bridge/capabilities/market_data.py` 等 | 仍保留兼容/辅助调用；行情数据主链已有 Go 实现，但不能称 Python 全面下线 |

## 3. 阶段一静态盘点结论

### 已实现且默认启用的主要能力

- 实时行情：东方财富、腾讯、新浪、网易、Pytdx。
- 日 K：东方财富、BaoStock、腾讯、新浪、网易、TickFlow、Pytdx。
- 股票搜索：东方财富、腾讯。
- 指数行情、市场宽度、板块排行：东方财富。
- 交易日历、财务：BaoStock。
- 涨跌停池、龙虎榜、公告、个股资金流、筹码、人气榜：东方财富为主要默认来源；妙想需独立配置。

### 已实现但默认关闭/需要权限验证

- TickFlow：实时行情、指数行情、市场宽度、板块排行。
- 同花顺：行情、日 K、指数、交易日历、人气榜、涨跌停池、龙虎榜、财务、估值、指数目录等。
- Tushare：证券基础资料、交易日历、交易状态、财务、估值、涨跌停池、龙虎榜、公告、指数目录及历史成分等。
- 雪球：人气榜。
- Yahoo Finance：海外行情能力，当前范围暂缓。

具体能力与开关以 `PYTHON_PROVIDER_GO_MIGRATION_MATRIX.md` 为准。不得只因为方法存在就直接打开配置。

## 4. 尚未完成事项

阶段三已进入开发和 CI 验收。市场情绪、全市场股票概览、Provider 主动能力探测/历史记录、板块涨跌幅/成交额/近五日涨幅排序，以及 Tushare 市场级资金流历史 API 均已有代码实现；市场级资金流仍需真实 Token 权限验收，严格 PIT 仍未完成。

| 优先级 | 事项 | 当前状态 | 下一步 |
|---|---|---|---|
| P0 | Python/Go 固定样本字段对账 | 缺少统一样本与完整旧版源码证据 | 固定旧版上游、建立脱敏样本和差异报告 |
| P1 | 已实现但关闭的 Provider 能力 | 代码可见，权限/真实可用性未完成统一验收 | 分 Provider、分 capability 验证后再启用 |
| P1 | 全市场批量概览 | 已实现分页概览、证券池范围、行情覆盖率和 MA 字段 | 继续核验真实全市场目录覆盖和并发负载 |
| P1 | 市场情绪历史 | 已实现当前快照采集与历史读取 | 继续验证 MariaDB 部署及真实交易日连续性 |
| P1 | 市场级资金流 | 已实现 Tushare `moneyflow_mkt_dc` 日期区间 API；不从个股资金流求和 | 使用有权限的 Tushare Token 做真实请求和字段单位核验；默认开关保持关闭 |
| P1 | 板块多维排行 | 已实现涨跌幅、成交额、近五日涨幅；近五日由六个交易日收盘价计算 | 继续真实接口验收；资金净流入维度暂缓，待确认可靠字段与完整板块全集 |
| P1 | Provider 主动诊断 | 已实现 Provider/capability 主动探测、超时隔离和安全摘要 | 补充真实环境权限覆盖并验证 MariaDB 历史持久化 |
| P2 | 诊断历史 | 已实现探测批次历史持久化接口；未配置存储时明确 persisted=false | 在 MariaDB 部署环境验证保留策略和查询性能 |
| P2 | 严格 PIT | 财务修订、历史成分、公司行动和 available_at 不完整 | 建立版本化历史数据和生效区间 |
| 暂缓 | 全球市场数据 | 不属于当前国内 A 股优先范围 | 国内数据验收后再评估 |

## 5. 执行顺序

1. 完成阶段一静态盘点文档和矩阵，确认所有“已实现/已关闭/缺失”的依据。
2. 进入阶段二，优先验证已有但关闭的国内 Provider，避免重复开发。
3. 对确实缺失的数据能力再新增 Provider 或服务层能力。
4. 完成阶段三市场级 API。
5. 最后集中进行固定样本对账、真实请求、部署和 CI 验收。

## 6. 当前验收状态

- [x] 核对目标分支远程 HEAD。
- [x] 清点 Provider 接口和主要实现文件。
- [x] 核对启动注册与默认 Provider 配置。
- [x] 清点 Python bridge 行情调用入口。
- [x] 形成迁移矩阵和缺口清单。
- [ ] 原 Python 上游版本固定。
- [ ] Python/Go 真实样本差分。
- [ ] 已关闭 Provider 的权限和真实请求验收。
- [x] 市场级资金流 Provider、服务层、API Handler、路由和主动探测入口。
- [ ] Tushare `moneyflow_mkt_dc` 真实 Token/120 积分权限验收及金额单位对账。
- [ ] 部署环境验收。

本文件只更新阶段状态，不替代真实运行验收记录。


## 7. 阶段三开发进度（2026-10-30）

| 能力 | 代码状态 | 质量边界/未完成项 |
|---|---|---|
| 市场情绪快照与历史 | 已实现 | 依赖 MariaDB 持久化；缺失的涨跌停/炸板计数保持 null，不合成零 |
| 全市场股票概览 | 已实现分页 API | 目录总数、行情覆盖率和 lower-bound 显式返回；真实大规模负载仍需验收 |
| Provider 主动能力探测 | 已实现 | 独立于业务请求；按 Provider/capability 返回结果，不输出凭据和原始 URL |
| Provider 探测历史 | 已实现持久化接口 | MariaDB 未配置时返回 persisted=false，历史查询不可用 |
| 板块涨跌幅/成交额排行 | 已实现 | 成交额必须为可验证 CNY；缺失金额不参与金额排行 |
| 板块近五日涨幅排行 | Provider/服务/API 已实现 | 拉取完整板块候选集，逐板块取六个日线收盘价计算；本轮修复 HTTP 与服务层对 `change_5d` 的参数拒绝，并补充端到端路由测试 |
| 市场级资金流 | 已实现代码路径 | Tushare `moneyflow_mkt_dc` 日期区间查询；缺失字段保留 null，未暴露精确快照时间时保持 degraded；真实 Token/单位对账待验收 |
| 严格 PIT | 未实现 | 财务修订、历史成分生效区间、公司行动与 available_at 仍需独立建设 |
